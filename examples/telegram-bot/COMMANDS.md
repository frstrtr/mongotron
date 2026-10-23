# Telegram Bot Commands Reference

## Quick Start

```bash
cd telegram-bot
./start.sh
```

## User Commands

### /start
Shows welcome message and available commands.

**Example:**
```
/start
```

**Response:**
```
👋 Welcome to Mongotron Monitor Bot!

This bot helps you monitor Tron blockchain addresses in real-time.
...
```

---

### /help
Displays help information (same as /start).

**Example:**
```
/help
```

---

### /monitor <address>
Start monitoring a Tron blockchain address. You'll receive real-time notifications for all transactions involving this address.

**Syntax:**
```
/monitor <TRON_ADDRESS>
```

**Example:**
```
/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

**What you'll receive:**
- Incoming transactions (received)
- Outgoing transactions (sent)
- Smart contract interactions
- Token transfers
- Contract events

**Notification format:**
```
🔔 New Transaction Detected
────────────────────────────────────────
📦 Block: 61115487
🔗 TX: 5e8381c0...114a0f574
📤 From: TKfUiqAG...c4p3yPqGf
📥 To: TXYZopYR...M5VkAeBf
💰 Amount: 0.000000 TRX
📋 Type: TriggerSmartContract

🔍 Smart Contract Details:
   ⚙️ Method: transfer(address,uint256)
   📍 Param Addresses:
      • TLVohkv4...yK9RdDFw8q
   💵 Token Amount: 6710000000

✅ Success

⏰ 20:44:15
```

---

### /stop_monitor <address>
Stop monitoring a specific address.

**Syntax:**
```
/stop_monitor <TRON_ADDRESS>
```

**Example:**
```
/stop_monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

**Response:**
```
✅ Stopped monitoring address:
TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

---

### /stop_all
Stop monitoring all addresses for your chat.

**Example:**
```
/stop_all
```

**Response:**
```
✅ Stopped all monitors (3 addresses):

• TKfUiqAG...c4p3yPqGf
• TXYZopYR...M5VkAeBf
• TLVohkv4...yK9RdDFw8q
```

---

### /list
List all addresses currently being monitored in your chat.

**Example:**
```
/list
```

**Response:**
```
📋 Active Monitors (3):

1. TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
2. TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf
3. TLVohkv4mQT5yK9RdDFw8q8SJtESQGfVAo

💡 Use /stop_monitor <address> to stop a specific monitor
💡 Use /stop_all to stop all monitors
```

---

## Transaction Details Explained

### Basic Transaction Info
- **📦 Block**: Block number where the transaction was included
- **🔗 TX**: Transaction hash (unique identifier)
- **📤 From**: Sender address
- **📥 To**: Recipient address
- **💰 Amount**: Amount transferred in TRX
- **📋 Type**: Transaction type (TransferContract, TriggerSmartContract, etc.)

### Smart Contract Details
When a transaction involves a smart contract:

- **⚙️ Method**: Function name called on the contract
  - Example: `transfer(address,uint256)` for token transfers
  
- **📍 Param Addresses**: Addresses extracted from function parameters
  - Shows up to 3 addresses involved
  - Useful for tracking token transfers and complex interactions
  
- **💵 Token Amount**: Amount of tokens being transferred
  - Shown in raw token units (use contract decimals to convert)

### Transaction Status
- **✅ Success**: Transaction completed successfully
- **❌ Failed**: Transaction failed or was reverted

---

## Use Cases

### Monitor Your Wallet
```
/monitor <YOUR_WALLET_ADDRESS>
```
Get instant notifications for all incoming and outgoing transactions.

### Track Token Transfers
Monitor a token contract to see all transfers:
```
/monitor TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t  # USDT contract
```

### Watch Exchanges
Monitor exchange hot wallets:
```
/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

### Track Smart Contracts
Monitor DeFi contracts for activity:
```
/monitor <CONTRACT_ADDRESS>
```

---

## Tips & Tricks

### Multiple Addresses
You can monitor multiple addresses simultaneously. Each will send independent notifications.

### Address Format
- Tron addresses always start with 'T'
- They are 34 characters long
- Example: `TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf`

### Notification Volume
Popular addresses (exchanges, tokens) can generate many notifications. Consider:
- Using separate chats for high-volume addresses
- Monitoring specific addresses rather than contracts
- Using filters (future feature)

### Performance
- Each monitor uses one WebSocket connection
- Monitors are chat-specific (each chat has its own monitors)
- Monitors automatically reconnect if disconnected

---

## Troubleshooting

### "Invalid address format"
- Check that address starts with 'T'
- Verify it's 34 characters long
- Make sure there are no extra spaces

### "Failed to register address"
- API server might be down
- Check network connection
- Verify API server URL in bot configuration

### "Connection lost"
- WebSocket connection was interrupted
- Monitor will be automatically stopped
- Start monitoring again with `/monitor`

### No notifications received
1. Check if monitor is active: `/list`
2. Verify the address has transactions
3. Check API server logs
4. Restart the monitor: `/stop_monitor` then `/monitor`

---

## Advanced Usage

### Running as System Service
```bash
# Copy service file
sudo cp mongotron-bot.service /etc/systemd/system/

# Edit service file if needed (adjust paths and user)
sudo nano /etc/systemd/system/mongotron-bot.service

# Enable and start service
sudo systemctl enable mongotron-bot
sudo systemctl start mongotron-bot

# Check status
sudo systemctl status mongotron-bot

# View logs
sudo journalctl -u mongotron-bot -f
```

### Environment Variables
Create `.env` file with:
```env
TELEGRAM_BOT_TOKEN=your_token_here
API_BASE_URL=http://localhost:8080
WS_BASE_URL=ws://localhost:8080
```

### Testing Configuration
```bash
python test_bot.py
```

This will verify:
- API server connectivity
- Telegram bot token validity
- Address registration capability

---

## Security

### Bot Token Security
- Never share your bot token
- Don't commit `.env` to git
- Rotate token if compromised (via @BotFather)

### Access Control
- Bot responds to any user by default
- Consider implementing user whitelist for private bots
- Add rate limiting for production

### Data Privacy
- Bot doesn't store user data permanently
- Monitors are cleared when bot restarts
- Chat IDs are used only for sending messages

---

## Development

### Project Structure
```
telegram-bot/
├── bot.py                    # Main bot code
├── test_bot.py              # Configuration test
├── requirements.txt         # Python dependencies
├── .env.example             # Environment template
├── .env                     # Your configuration (gitignored)
├── .gitignore              # Git ignore rules
├── start.sh                # Quick start script
├── mongotron-bot.service   # Systemd service file
├── README.md               # Main documentation
└── COMMANDS.md             # This file
```

### Adding New Commands
1. Define command handler with `@dp.message(Command("command_name"))`
2. Extract parameters from `message.text`
3. Implement functionality
4. Send response with `message.answer()`

### Modifying Event Format
Edit the `format_event()` function in `bot.py` to customize notification appearance.

---

## Support

For issues or questions:
1. Check this documentation
2. Review bot logs
3. Check API server logs
4. Test with `test_bot.py`

## Future Enhancements

Potential features:
- Filters (minimum amount, transaction type)
- Custom alerts
- Statistics and summaries
- Multi-language support
- Inline buttons for quick actions
- Address nicknames/labels
