# Bug Fix: Double Subscription Deletion (500 Error)

## Issue Summary

**Problem**: When stopping the event monitor with Ctrl+C, subscription deletion was called twice, causing a 500 error on the second attempt.

**Symptoms**:
```
🧹 Cleaning up subscription: sub_fa77ad23-289
✅ Subscription deleted
🧹 Cleaning up subscription: sub_fa77ad23-289
⚠️  Failed to delete subscription: 500
```

**Server Logs**:
```
[23:21:57] 200 -    1.838099ms DELETE /api/v1/subscriptions/sub_fa77ad23-289
[23:21:57] 500 -      11.941µs DELETE /api/v1/subscriptions/sub_fa77ad23-289
```

## Root Cause

### Double-Call Flow

When user presses **Ctrl+C**:

1. **Signal Handler** (`signal_handler()`) is triggered
   - Calls `self.stop()` → Deletes subscription → **200 OK**
   - Calls `sys.exit(0)`

2. **Finally Block** in `run()` executes (Python cleanup)
   - Calls `self.stop()` **again**
   - Tries to delete already-deleted subscription → **500 Error**

### Code Flow

**event_monitor.py**:

```python
def signal_handler(self, signum, frame):
    """Handle shutdown signals"""
    print("\n\n🛑 Shutting down gracefully...")
    self.stop()        # <-- First call (SUCCESS)
    sys.exit(0)

def run(self, address: str, filters: dict = None):
    try:
        while self.running:
            time.sleep(1)
    except KeyboardInterrupt:
        pass
    finally:
        self.stop()        # <-- Second call (500 ERROR)
```

### Server-Side Error

**internal/subscription/manager.go** (Line 152-154):

```go
func (m *Manager) Unsubscribe(subscriptionID string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    wrapper, exists := m.monitors[subscriptionID]
    if !exists {
        return fmt.Errorf("subscription not found")  // <-- Returns error on second call
    }
    
    // ... delete from map
    delete(m.monitors, subscriptionID)
}
```

**Why 500?**

**internal/api/handlers/subscription.go**:

```go
func (h *SubscriptionHandler) DeleteSubscription(c *fiber.Ctx) error {
    // ...
    if err := h.manager.Unsubscribe(subscriptionID); err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
            Error:   "unsubscribe_failed",
            Message: err.Error(),
        })
    }
}
```

## Solution

Add **idempotency guard** to `stop()` method to prevent double-execution.

### Implementation

**event_monitor.py** (Lines 394-423):

```python
def stop(self):
    """Stop monitoring and cleanup"""
    # Guard against double-call
    if hasattr(self, '_stopped') and self._stopped:
        return
    self._stopped = True
    
    if self.ws and self.running:
        print("\n🛑 Closing WebSocket connection...")
        self.ws.close()
        time.sleep(1)
    
    if self.subscription_id:
        print(f"🧹 Cleaning up subscription: {self.subscription_id}")
        try:
            response = requests.delete(
                f"{self.api_base}/subscriptions/{self.subscription_id}",
                timeout=10
            )
            if response.status_code == 200:
                print("✅ Subscription deleted")
            else:
                print(f"⚠️  Failed to delete subscription: {response.status_code}")
        except Exception as e:
            print(f"⚠️  Error deleting subscription: {e}")
    
    # Show log file location
    if hasattr(self, 'log_file') and self.event_count > 0:
        print(f"\n📝 Events saved to: {self.log_file}")
        print(f"   Total events logged: {self.event_count}")
```

### How It Works

1. **First call** to `stop()`:
   - `_stopped` attribute doesn't exist or is `False`
   - Sets `_stopped = True`
   - Proceeds with cleanup (deletes subscription)

2. **Second call** to `stop()`:
   - `_stopped` is `True`
   - **Returns immediately** (no-op)
   - No DELETE request sent

## Verification

### Before Fix

```bash
# Server logs show double deletion
[23:21:57] 200 -    1.838099ms DELETE /api/v1/subscriptions/sub_fa77ad23-289
[23:21:57] 500 -      11.941µs DELETE /api/v1/subscriptions/sub_fa77ad23-289
```

### After Fix

```bash
# Server logs show single deletion
[23:35:21] 200 -    1.821782ms DELETE /api/v1/subscriptions/sub_b8a4d9e4-161
```

### Test Output

```
🧹 Cleaning up subscription: sub_b8a4d9e4-161
✅ Subscription deleted

📝 Events saved to: events_20251005_233510.log
   Total events logged: 0
```

✅ **No error message**
✅ **Single deletion**
✅ **Clean shutdown**

## Impact

### Benefits

1. ✅ **No false errors** - Clean shutdown without scary 500 messages
2. ✅ **Server efficiency** - Reduces unnecessary API calls
3. ✅ **Better UX** - Professional, error-free output
4. ✅ **Idempotent** - Safe to call `stop()` multiple times

### Side Effects

**None** - The fix is purely defensive and doesn't change functionality:
- First call still works exactly as before
- Additional calls are safely ignored
- No impact on normal operation

## Alternative Solutions Considered

### 1. Remove Signal Handler
```python
# Don't set up signal handler, rely only on finally block
# ❌ REJECTED: Less control over shutdown sequence
```

### 2. Remove sys.exit() from Signal Handler
```python
def signal_handler(self, signum, frame):
    self.stop()
    # Don't call sys.exit(0)
# ❌ REJECTED: May not exit cleanly in all scenarios
```

### 3. Flag-based Coordination
```python
# Set a flag in signal handler, check in finally
# ❌ REJECTED: More complex than needed
```

### 4. Idempotency Guard (CHOSEN)
```python
# Simple, safe, doesn't change control flow
# ✅ ACCEPTED: Minimal change, maximum safety
```

## Testing

### Manual Test
```bash
cd /home/user0/Github/mongotron
source venv/bin/activate

# Start monitor
python event_monitor.py

# Press Ctrl+C after a few seconds
# Verify: Only one "Subscription deleted" message
# Verify: No "Failed to delete subscription: 500" error
```

### Server Log Check
```bash
# Check for subscription deletion in logs
tail -100 /tmp/api-server.log | grep DELETE | grep subscriptions

# Should see only ONE 200 response per subscription
# Should NOT see any 500 responses
```

## Related Code

### Files Modified
- `event_monitor.py` (Lines 394-423)

### Files Analyzed
- `internal/subscription/manager.go` (Lines 146-175)
- `internal/api/handlers/subscription.go` (Lines 146-165)

### Test Files Referenced
- None (manual testing performed)

## Date
October 5, 2025

## Status
✅ **FIXED AND VERIFIED**
