# Telegram Bot - Complete Setup Guide

## 🎉 What Was Created

A complete Telegram bot for monitoring Tron blockchain addresses in real-time!

### 📁 Project Structure

```
telegram-bot/
├── bot.py                    # Main bot application (458 lines)
├── test_bot.py              # Configuration testing script
├── requirements.txt         # Python dependencies
├── .env.example             # Environment configuration template
├── .gitignore              # Git ignore rules
├── start.sh                # Quick start script (executable)
├── mongotron-bot.service   # Systemd service file
├── README.md               # Complete documentation
└── COMMANDS.md             # Command reference guide
```

## 🚀 Quick Start (3 Steps)

### 1. Get Your Bot Token
1. Open Telegram and search for [@BotFather](https://t.me/botfather)
2. Send `/newbot` command
3. Follow instructions and get your token

### 2. Configure the Bot
```bash
cd telegram-bot
cp .env.example .env
nano .env  # Add your bot token
```

### 3. Start the Bot
```bash
./start.sh
```

That's it! 🎊

## 📱 Bot Commands

| Command | Description | Example |
|---------|-------------|---------|
| `/start` | Show welcome message | `/start` |
| `/monitor <address>` | Start monitoring an address | `/monitor TKfUiq...` |
| `/stop_monitor <address>` | Stop monitoring an address | `/stop_monitor TKfUiq...` |
| `/stop_all` | Stop all monitors | `/stop_all` |
| `/list` | List active monitors | `/list` |
| `/help` | Show help | `/help` |

## ✨ Features

### Real-Time Monitoring
- ✅ WebSocket-based instant notifications
- ✅ Monitor multiple addresses simultaneously
- ✅ Each address gets independent monitoring

### Rich Transaction Details
- 📦 Block number
- 🔗 Transaction hash
- 📤 From/To addresses
- 💰 Amount in TRX
- 📋 Transaction type
- ✅/❌ Success status

### Smart Contract Decoding
- 🔍 Function name (e.g., `transfer(address,uint256)`)
- 📍 Parameter addresses extracted
- 💵 Token amounts
- ⚙️ Method signatures

### User-Friendly
- 🎯 Simple commands
- 📱 Formatted notifications
- 🔔 Instant alerts
- 📊 Easy address management

## 🔧 Manual Setup (Alternative)

If you prefer manual setup instead of using `start.sh`:

```bash
# 1. Create virtual environment
python3 -m venv venv
source venv/bin/activate

# 2. Install dependencies
pip install -r requirements.txt

# 3. Configure
cp .env.example .env
nano .env  # Set TELEGRAM_BOT_TOKEN

# 4. Test configuration
python test_bot.py

# 5. Start bot
python bot.py
```

## 📋 Requirements

- Python 3.8 or higher
- Mongotron API server running (default: http://localhost:8080)
- Telegram Bot Token from @BotFather
- Internet connection

## 🔍 Testing

### Test Configuration
```bash
python test_bot.py
```

This will check:
- ✅ API server connectivity
- ✅ Telegram bot token validity
- ✅ Address registration capability

### Example Output
```
============================================================
Mongotron Telegram Bot - Configuration Test
============================================================
Testing API connection...
API Base URL: http://localhost:8080
✅ API server is reachable
   Response: {'status': 'ok'}

Testing Telegram bot token...
Token: 123456789:...ABCDEF123
✅ Bot token is valid
   Bot name: @YourBotName
   Bot ID: 123456789

Testing address registration...
✅ Successfully registered address: TKfUiqAG...
   Response: {'success': True}
```

## 🎯 Example Usage

### 1. Start the Bot
Open your bot in Telegram and send:
```
/start
```

### 2. Monitor an Address
```
/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

You'll see:
```
✅ Now monitoring address:
TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf

You'll receive notifications for all transactions related to this address.
```

### 3. Receive Notifications
When a transaction occurs:
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

### 4. Check Active Monitors
```
/list
```

Response:
```
📋 Active Monitors (1):

1. TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf

💡 Use /stop_monitor <address> to stop a specific monitor
💡 Use /stop_all to stop all monitors
```

### 5. Stop Monitoring
```
/stop_monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

Or stop all:
```
/stop_all
```

## 🔐 Security

### Protect Your Token
- ✅ `.env` file is git-ignored
- ✅ Never commit your bot token
- ✅ Don't share your token publicly
- ✅ Rotate if compromised via @BotFather

### Bot Access
- Bot responds to any Telegram user
- Consider implementing user whitelist for private use
- Add rate limiting for production

## 🏃 Running as Service

### Install as Systemd Service
```bash
# Copy service file
sudo cp mongotron-bot.service /etc/systemd/system/

# Edit if needed (adjust user and paths)
sudo nano /etc/systemd/system/mongotron-bot.service

# Enable and start
sudo systemctl enable mongotron-bot
sudo systemctl start mongotron-bot

# Check status
sudo systemctl status mongotron-bot

# View logs
sudo journalctl -u mongotron-bot -f
```

### Service Management
```bash
# Start
sudo systemctl start mongotron-bot

# Stop
sudo systemctl stop mongotron-bot

# Restart
sudo systemctl restart mongotron-bot

# Status
sudo systemctl status mongotron-bot

# Logs
sudo journalctl -u mongotron-bot -f
```

## 🐛 Troubleshooting

### Bot Won't Start

**Issue: "TELEGRAM_BOT_TOKEN not found"**
```bash
# Solution: Configure .env file
cp .env.example .env
nano .env  # Add your token
```

**Issue: "Import aiogram could not be resolved"**
```bash
# Solution: Install dependencies
pip install -r requirements.txt
```

### API Connection Failed

**Issue: "Failed to connect to API"**
```bash
# Check if API server is running
curl http://localhost:8080/health

# Start API server
cd /home/user0/Github/mongotron
./bin/api-server
```

### No Notifications

**Check monitor status:**
```
/list
```

**Verify address has transactions:**
- Check on TronScan
- Ensure address format is correct

**Restart monitor:**
```
/stop_monitor <address>
/monitor <address>
```

## 📚 Documentation

- **README.md** - Complete project documentation
- **COMMANDS.md** - Detailed command reference with examples
- **This file** - Quick setup guide

## 🔧 Dependencies

```
aiogram==3.13.1        # Telegram Bot framework
aiohttp==3.10.5        # HTTP client
python-dotenv==1.0.0   # Environment variables
websockets==13.1       # WebSocket client
```

## 🎨 Customization

### Change Notification Format
Edit `format_event()` function in `bot.py`:
```python
def format_event(event: dict) -> str:
    # Customize your notification format here
    pass
```

### Add New Commands
Add command handler in `bot.py`:
```python
@dp.message(Command("your_command"))
async def cmd_your_command(message: Message):
    # Your command logic
    pass
```

### Modify Monitoring Logic
Edit `monitor_address()` function in `bot.py`

## 📊 Architecture

```
┌─────────────┐      REST API       ┌──────────────┐
│             │◄───────────────────►│              │
│  Telegram   │                     │  Mongotron   │
│    Bot      │      WebSocket      │  API Server  │
│             │◄───────────────────►│              │
└─────────────┘                     └──────────────┘
      │                                     │
      │                                     │
      ▼                                     ▼
┌─────────────┐                     ┌──────────────┐
│  Telegram   │                     │   MongoDB    │
│   Users     │                     │   Database   │
└─────────────┘                     └──────────────┘
```

## 🎁 What You Get

1. **Complete Bot Application** - Ready to use
2. **Test Script** - Verify configuration
3. **Auto Setup Script** - One-command installation
4. **Service File** - Run as system service
5. **Full Documentation** - Complete guides
6. **Security** - .gitignore configured

## 🚀 Next Steps

1. **Get Bot Token** from @BotFather
2. **Configure** `.env` file
3. **Run** `./start.sh`
4. **Test** with `/start` in Telegram
5. **Monitor** your first address!

## 💡 Use Cases

- 📊 **Personal Wallet Monitoring** - Track your own transactions
- 🏦 **Exchange Monitoring** - Watch exchange wallets
- 🪙 **Token Contract Monitoring** - Track token transfers
- 🔍 **DeFi Contract Monitoring** - Watch smart contract activity
- 📈 **Portfolio Tracking** - Monitor multiple addresses

## ✨ Features Highlights

✅ Real-time WebSocket notifications
✅ Multi-address monitoring
✅ Smart contract decoding
✅ Token transfer detection
✅ Parameter address extraction
✅ User-friendly formatting
✅ Easy command interface
✅ Automatic reconnection
✅ Clean error handling
✅ Production-ready

## 🎉 Ready to Go!

Your Telegram bot is ready to monitor the Tron blockchain!

```bash
cd telegram-bot
./start.sh
```

Happy monitoring! 🚀
