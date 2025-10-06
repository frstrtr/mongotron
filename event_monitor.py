#!/usr/bin/env python3
"""
MongoTron Event Monitor
Subscribes to blockchain events and displays real-time updates via WebSocket
"""

import requests
import json
import time
import sys
import signal
import logging
import hashlib
import base58
from datetime import datetime
from websocket import WebSocketApp
import threading

# USDT contract on Tron Nile Testnet
# Note: This is a testnet USDT contract. For mainnet, use: TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t
USDT_CONTRACT = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"  # Tron Nile Testnet USDT

import threading

# USDT contract on Tron Nile Testnet
# Note: This is a testnet USDT contract. For mainnet, use: TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t
USDT_CONTRACT = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"  # Tron Nile Testnet USDT

def hex_to_tron_address(hex_address: str) -> str:
    """
    Convert hex address to Tron base58 address
    
    Args:
        hex_address: Hex address (with or without 0x prefix, with or without 41 prefix)
    
    Returns:
        Base58 encoded Tron address (T...)
    """
    try:
        # Remove 0x prefix if present
        if hex_address.startswith('0x') or hex_address.startswith('0X'):
            hex_address = hex_address[2:]
        
        # Remove padding zeros if present (from topics)
        hex_address = hex_address.lstrip('0')
        
        # Add 41 prefix if not present (Tron mainnet/testnet prefix)
        if not hex_address.startswith('41'):
            hex_address = '41' + hex_address
        
        # Ensure even length
        if len(hex_address) % 2 != 0:
            hex_address = '0' + hex_address
        
        # Convert hex to bytes
        addr_bytes = bytes.fromhex(hex_address)
        
        # Calculate checksum (double SHA256)
        hash1 = hashlib.sha256(addr_bytes).digest()
        hash2 = hashlib.sha256(hash1).digest()
        checksum = hash2[:4]
        
        # Append checksum and encode to base58
        addr_with_checksum = addr_bytes + checksum
        base58_addr = base58.b58encode(addr_with_checksum).decode('utf-8')
        
        return base58_addr
    except Exception as e:
        # If conversion fails, return original hex
        return f"{hex_address} (conversion failed: {e})"

class EventMonitor:
    """Monitor blockchain events for a specific address"""
    
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.api_base = f"{base_url}/api/v1"
        self.subscription_id = None
        self.ws = None
        self.running = False
        self.event_count = 0
        self.start_time = None
        
        # Setup file logger for events
        self.setup_file_logger()
        
        # Setup signal handler for graceful shutdown
        signal.signal(signal.SIGINT, self.signal_handler)
        signal.signal(signal.SIGTERM, self.signal_handler)
    
    def display_smart_contract_info(self, smart_contract: dict):
        """Display decoded smart contract information in human-readable format"""
        print(f"\n{'─'*80}")
        print(f"🔍 SMART CONTRACT CALL DECODED:")
        print(f"{'─'*80}")
        
        method_name = smart_contract.get("methodName", "Unknown")
        method_sig = smart_contract.get("methodSignature", "N/A")
        addresses = smart_contract.get("addresses", [])
        parameters = smart_contract.get("parameters", {})
        amount = smart_contract.get("amount")
        
        # Display method information
        print(f"📝 Function:     {method_name}")
        print(f"🔑 Signature:    0x{method_sig}")
        
        # Parse and display function parameters in human-readable format
        if "transfer(address,uint256)" in method_name:
            print(f"\n💸 TOKEN TRANSFER:")
            if len(addresses) > 0:
                recipient = addresses[0]
                recipient_readable = hex_to_tron_address(recipient)
                print(f"   To (hex):     {recipient}")
                print(f"   To:           {recipient_readable}")
            if amount:
                # Try to format amount (assuming USDT with 6 decimals)
                try:
                    amount_int = int(amount)
                    amount_formatted = amount_int / 1_000_000
                    print(f"   Amount:       {amount_formatted:,.6f} tokens")
                    print(f"   Amount (raw): {amount}")
                except:
                    print(f"   Amount:       {amount}")
        
        elif "transferFrom(address,address,uint256)" in method_name:
            print(f"\n💸 TOKEN TRANSFER (Approved):")
            if len(addresses) >= 2:
                from_addr = addresses[0]
                to_addr = addresses[1]
                from_readable = hex_to_tron_address(from_addr)
                to_readable = hex_to_tron_address(to_addr)
                print(f"   From (hex):   {from_addr}")
                print(f"   From:         {from_readable}")
                print(f"   To (hex):     {to_addr}")
                print(f"   To:           {to_readable}")
            if amount:
                try:
                    amount_int = int(amount)
                    amount_formatted = amount_int / 1_000_000
                    print(f"   Amount:       {amount_formatted:,.6f} tokens")
                    print(f"   Amount (raw): {amount}")
                except:
                    print(f"   Amount:       {amount}")
        
        elif "approve(address,uint256)" in method_name:
            print(f"\n✅ TOKEN APPROVAL:")
            if len(addresses) > 0:
                spender = addresses[0]
                spender_readable = hex_to_tron_address(spender)
                print(f"   Spender (hex): {spender}")
                print(f"   Spender:       {spender_readable}")
            if amount:
                try:
                    amount_int = int(amount)
                    amount_formatted = amount_int / 1_000_000
                    print(f"   Allowance:     {amount_formatted:,.6f} tokens")
                    print(f"   Allowance (raw): {amount}")
                except:
                    print(f"   Allowance:     {amount}")
        
        elif "balanceOf(address)" in method_name:
            print(f"\n💰 BALANCE QUERY:")
            if len(addresses) > 0:
                owner = addresses[0]
                owner_readable = hex_to_tron_address(owner)
                print(f"   Owner (hex):  {owner}")
                print(f"   Owner:        {owner_readable}")
        
        elif "allowance(address,address)" in method_name:
            print(f"\n🔍 ALLOWANCE QUERY:")
            if len(addresses) >= 2:
                owner = addresses[0]
                spender = addresses[1]
                owner_readable = hex_to_tron_address(owner)
                spender_readable = hex_to_tron_address(spender)
                print(f"   Owner (hex):  {owner}")
                print(f"   Owner:        {owner_readable}")
                print(f"   Spender (hex): {spender}")
                print(f"   Spender:      {spender_readable}")
        
        else:
            # Generic display for unknown methods
            print(f"\n📋 PARAMETERS:")
            if addresses:
                print(f"   Addresses extracted:")
                for i, addr in enumerate(addresses):
                    readable = hex_to_tron_address(addr)
                    print(f"      [{i}] {addr}")
                    print(f"          {readable}")
            if parameters:
                print(f"   Other parameters:")
                for key, value in parameters.items():
                    print(f"      {key}: {value}")
            if amount:
                print(f"   Amount: {amount}")
        
        print(f"{'─'*80}")
    
    def setup_file_logger(self):
        """Setup file logger to save all events to a file"""
        log_filename = f"events_{datetime.now().strftime('%Y%m%d_%H%M%S')}.log"
        self.log_file = log_filename
        
        # Create file logger
        self.file_logger = logging.getLogger('event_logger')
        self.file_logger.setLevel(logging.INFO)
        
        # Create file handler
        file_handler = logging.FileHandler(log_filename)
        file_handler.setLevel(logging.INFO)
        
        # Create formatter
        formatter = logging.Formatter('%(asctime)s - %(message)s')
        file_handler.setFormatter(formatter)
        
        # Add handler to logger
        self.file_logger.addHandler(file_handler)
        
        print(f"📝 Logging events to file: {log_filename}")
    
    def signal_handler(self, signum, frame):
        """Handle shutdown signals"""
        print("\n\n🛑 Shutting down gracefully...")
        self.stop()
        sys.exit(0)
    
    def create_subscription(self, address: str, filters: dict = None) -> bool:
        """Create a subscription for the given address"""
        print(f"📋 Creating subscription for address: {address}")
        
        payload = {
            "address": address,
            "filters": filters or {},
            "startBlock": -1  # Start from latest block
        }
        
        try:
            response = requests.post(
                f"{self.api_base}/subscriptions",
                json=payload,
                timeout=10
            )
            
            if response.status_code in [200, 201]:
                data = response.json()
                self.subscription_id = data.get("subscriptionId")
                print(f"✅ Subscription created: {self.subscription_id}")
                print(f"   Network: {data.get('network', 'tron-nile')}")
                print(f"   Active: {data.get('status', 'active')}")
                return True
            else:
                print(f"❌ Failed to create subscription: {response.status_code}")
                print(f"   Error: {response.text}")
                return False
                
        except Exception as e:
            print(f"❌ Error creating subscription: {e}")
            return False
    
    def on_ws_open(self, ws):
        """WebSocket connection opened"""
        self.running = True
        self.start_time = datetime.now()
        print(f"\n🔌 WebSocket connected!")
        print(f"⏰ Started at: {self.start_time.strftime('%Y-%m-%d %H:%M:%S')}")
        print(f"\n{'='*80}")
        print(f"{'MONITORING EVENTS':-^80}")
        print(f"{'='*80}\n")
    
    def on_ws_message(self, ws, message):
        """Handle incoming WebSocket message"""
        try:
            data = json.loads(message)
            event_type = data.get("type", "unknown")
            
            if event_type == "connected":
                print("✅ Connection confirmed by server")
                print(f"   Subscription: {data.get('subscription_id', 'N/A')}\n")
                
            elif event_type == "event":
                self.event_count += 1
                self.display_event(data, self.event_count)
                
            elif event_type == "error":
                print(f"\n⚠️  ERROR from server:")
                print(f"   {data.get('message', 'Unknown error')}\n")
                
            else:
                # Handle transaction events (capital letter fields at root)
                if "TransactionID" in data or "TransactionHash" in data:
                    self.event_count += 1
                    self.display_transaction_event(data, self.event_count)
                else:
                    print(f"\n📨 Received message (type: {event_type}):")
                    print(json.dumps(data, indent=2))
                    print()
                
        except json.JSONDecodeError:
            print(f"\n⚠️  Received non-JSON message: {message}\n")
        except Exception as e:
            print(f"\n❌ Error processing message: {e}\n")
    
    def display_event(self, event_data: dict, count: int):
        """Display event in a formatted way with full details"""
        event = event_data.get("data", {})
        
        # Log to file immediately
        self.file_logger.info(f"EVENT #{count} - {json.dumps(event_data, indent=2)}")
        
        # Extract event details
        event_id = event.get("id", "N/A")
        event_name = event.get("event_name", "Unknown")
        contract = event.get("contract_address", "N/A")
        tx_hash = event.get("transaction_hash", "N/A")
        block_number = event.get("block_number", "N/A")
        timestamp = event.get("timestamp", 0)
        topics = event.get("topics", [])
        data_hex = event.get("data", "")
        
        # Format timestamp
        if timestamp:
            dt = datetime.fromtimestamp(timestamp)
            time_str = dt.strftime('%Y-%m-%d %H:%M:%S')
        else:
            time_str = "N/A"
        
        # Display event header
        print(f"\n{'='*80}")
        print(f"🔔 EVENT #{count} - {event_name}")
        print(f"{'='*80}")
        print(f"📍 ID:          {event_id}")
        print(f"📄 Contract:    {contract}")
        print(f"🔗 TX Hash:     {tx_hash[:20]}...{tx_hash[-20:] if len(tx_hash) > 40 else ''}")
        print(f"📦 Block:       {block_number}")
        print(f"⏰ Time:        {time_str}")
        
        # Display smart contract decoded information if available
        smart_contract = event.get("smartContract")
        if smart_contract:
            self.display_smart_contract_info(smart_contract)
        
        # Log full raw event data as JSON
        print(f"\n{'─'*80}")
        print(f"📋 FULL EVENT DATA (JSON):")
        print(f"{'─'*80}")
        print(json.dumps(event, indent=2, sort_keys=False))
        print(f"{'─'*80}")
        
        # Log the complete message structure
        print(f"\n{'─'*80}")
        print(f"📦 COMPLETE MESSAGE STRUCTURE:")
        print(f"{'─'*80}")
        print(json.dumps(event_data, indent=2, sort_keys=False))
        print(f"{'─'*80}")
        
        # Display parsed fields for easy reading
        print(f"\n{'─'*80}")
        print(f"📊 PARSED FIELDS:")
        print(f"{'─'*80}")
        
        if topics:
            print(f"\n📋 Topics ({len(topics)}):")
            for i, topic in enumerate(topics):
                print(f"   [{i}] {topic}")
        
        if data_hex:
            print(f"\n💾 Hex Data:")
            # Display first 100 chars of data
            if len(data_hex) > 100:
                print(f"   {data_hex[:100]}...")
                print(f"   (Total length: {len(data_hex)} chars)")
            else:
                print(f"   {data_hex}")
        
        # Try to decode Transfer event
        if event_name == "Transfer" and len(topics) >= 3:
            print(f"\n� DECODED TRANSFER:")
            from_addr = topics[1] if len(topics) > 1 else "N/A"
            to_addr = topics[2] if len(topics) > 2 else "N/A"
            print(f"   From: {from_addr}")
            print(f"   To:   {to_addr}")
            if data_hex and len(data_hex) >= 64:
                # Try to decode amount (hex to decimal)
                try:
                    amount_hex = data_hex[:64]
                    amount = int(amount_hex, 16)
                    # USDT has 6 decimals
                    amount_usdt = amount / 1_000_000
                    print(f"   Amount: {amount_usdt:,.6f} USDT")
                except:
                    print(f"   Amount: (could not decode)")
        
        print(f"\n{'='*80}\n")
    
    def display_transaction_event(self, event_data: dict, count: int):
        """Display transaction event with human-readable addresses and smart contract decoding"""
        # Log to file immediately
        self.file_logger.info(f"TRANSACTION EVENT #{count} - {json.dumps(event_data, indent=2)}")
        
        # Extract transaction details
        tx_id = event_data.get("TransactionID", event_data.get("TransactionHash", "N/A"))
        block_num = event_data.get("BlockNumber", "N/A")
        block_timestamp = event_data.get("BlockTimestamp", 0)
        from_hex = event_data.get("From", "N/A")
        to_hex = event_data.get("To", "N/A")
        amount = event_data.get("Amount", 0)
        contract_type = event_data.get("ContractType", "Unknown")
        success = event_data.get("Success", False)
        event_data_dict = event_data.get("EventData", {})
        
        # Convert addresses to human-readable format
        from_readable = hex_to_tron_address(from_hex) if from_hex != "N/A" else "N/A"
        to_readable = hex_to_tron_address(to_hex) if to_hex != "N/A" else "N/A"
        
        # Format timestamp
        if block_timestamp:
            dt = datetime.fromtimestamp(block_timestamp / 1000)  # Convert ms to seconds
            time_str = dt.strftime('%Y-%m-%d %H:%M:%S')
        else:
            time_str = "N/A"
        
        # Display event header
        print(f"\n{'='*80}")
        print(f"🔔 TRANSACTION EVENT #{count} - {contract_type}")
        print(f"{'='*80}")
        print(f"📍 TX ID:       {tx_id[:20]}...{tx_id[-20:] if len(tx_id) > 40 else ''}")
        print(f"📦 Block:       {block_num}")
        print(f"⏰ Time:        {time_str}")
        print(f"✅ Success:     {success}")
        
        # Display addresses with both hex and readable formats
        print(f"\n{'─'*80}")
        print(f"📊 ADDRESSES:")
        print(f"{'─'*80}")
        print(f"   From (hex):  {from_hex}")
        print(f"   From:        {from_readable}")
        print(f"   To (hex):    {to_hex}")
        print(f"   To:          {to_readable}")
        if amount > 0:
            print(f"   Amount:      {amount}")
        
        # Display smart contract decoded information if available
        smart_contract = event_data_dict.get("smartContract")
        if smart_contract:
            self.display_smart_contract_info(smart_contract)
        
        # Log full raw event data as JSON
        print(f"\n{'─'*80}")
        print(f"📋 FULL EVENT DATA (JSON):")
        print(f"{'─'*80}")
        print(json.dumps(event_data, indent=2, sort_keys=False))
        print(f"{'─'*80}")
        
        print(f"\n{'='*80}\n")
    
    def on_ws_error(self, ws, error):
        """Handle WebSocket error"""
        print(f"\n❌ WebSocket error: {error}\n")
    
    def on_ws_close(self, ws, close_status_code, close_msg):
        """WebSocket connection closed"""
        self.running = False
        print(f"\n{'='*80}")
        print(f"🔌 WebSocket closed")
        if close_status_code:
            print(f"   Status code: {close_status_code}")
        if close_msg:
            print(f"   Message: {close_msg}")
        
        if self.start_time:
            duration = datetime.now() - self.start_time
            print(f"\n📊 Session Summary:")
            print(f"   Duration: {duration}")
            print(f"   Events received: {self.event_count}")
            if duration.total_seconds() > 0:
                rate = self.event_count / duration.total_seconds()
                print(f"   Rate: {rate:.2f} events/second")
        print(f"{'='*80}\n")
    
    def connect_websocket(self) -> bool:
        """Connect to WebSocket stream"""
        if not self.subscription_id:
            print("❌ No subscription ID available")
            return False
        
        ws_url = f"ws://localhost:8080/api/v1/events/stream/{self.subscription_id}"
        print(f"\n🔌 Connecting to WebSocket...")
        print(f"   URL: {ws_url}")
        
        try:
            self.ws = WebSocketApp(
                ws_url,
                on_open=self.on_ws_open,
                on_message=self.on_ws_message,
                on_error=self.on_ws_error,
                on_close=self.on_ws_close
            )
            
            # Run WebSocket in a separate thread
            ws_thread = threading.Thread(target=self.ws.run_forever)
            ws_thread.daemon = True
            ws_thread.start()
            
            # Wait a bit for connection
            time.sleep(1)
            
            return True
            
        except Exception as e:
            print(f"❌ Failed to connect WebSocket: {e}")
            return False
    
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
    
    def run(self, address: str, filters: dict = None):
        """Main monitoring loop"""
        print("\n" + "="*80)
        print(f"{'MongoTron Event Monitor':^80}")
        print("="*80 + "\n")
        
        # Create subscription
        if not self.create_subscription(address, filters):
            return False
        
        # Connect WebSocket
        if not self.connect_websocket():
            self.stop()
            return False
        
        # Keep running
        print("\n💡 Press Ctrl+C to stop monitoring\n")
        
        try:
            while self.running:
                time.sleep(1)
        except KeyboardInterrupt:
            pass
        finally:
            self.stop()
        
        return True


def main():
    """Main entry point"""
    # Parse command line arguments
    import argparse
    
    parser = argparse.ArgumentParser(
        description='Monitor blockchain events in real-time',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Monitor USDT contract (default)
  python event_monitor.py
  
  # Monitor a specific address
  python event_monitor.py --address TRX9aKj8VrqEJMvM7vvkA2B3wYGpwj5YSj
  
  # Connect to a different server
  python event_monitor.py --url http://staging-server:8080
  
  # Monitor with filters (JSON)
  python event_monitor.py --filters '{"event_name": "Transfer"}'
        """
    )
    
    parser.add_argument(
        '--address',
        default=USDT_CONTRACT,
        help=f'Contract address to monitor (default: USDT Nile Testnet {USDT_CONTRACT})'
    )
    
    parser.add_argument(
        '--url',
        default='http://localhost:8080',
        help='MongoTron API base URL (default: http://localhost:8080)'
    )
    
    parser.add_argument(
        '--filters',
        type=str,
        help='Event filters as JSON string'
    )
    
    args = parser.parse_args()
    
    # Parse filters if provided
    filters = None
    if args.filters:
        try:
            filters = json.loads(args.filters)
        except json.JSONDecodeError as e:
            print(f"❌ Invalid JSON in --filters: {e}")
            return 1
    
    # Create and run monitor
    monitor = EventMonitor(args.url)
    
    try:
        success = monitor.run(args.address, filters)
        return 0 if success else 1
    except Exception as e:
        print(f"\n❌ Unexpected error: {e}")
        import traceback
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
