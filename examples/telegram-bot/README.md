# Mongotron Telegram Bot

A Telegram bot for monitoring Tron blockchain addresses in real-time using the Mongotron API.

## Features

- 📊 Real-time address monitoring via WebSocket
- 🔔 Instant notifications for all transactions
- 💰 Transaction details including amounts, addresses, and types
- 🔍 Smart contract decoding with method names and parameters
- 📱 Multiple address monitoring support
- 🎯 Easy-to-use commands

## Prerequisites

- Python 3.8+
- Mongotron API server running (default: http://localhost:8080)
- Telegram Bot Token (get from [@BotFather](https://t.me/botfather))

## Installation

1. **Navigate to the telegram-bot directory:**
   ```bash
   cd telegram-bot
   ```

2. **Create a virtual environment:**
   ```bash
   python -m venv venv
   source venv/bin/activate  # On Linux/Mac
   # or
   venv\Scripts\activate  # On Windows
   ```

3. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

4. **Configure the bot:**
   ```bash
   cp .env.example .env
   ```
   
   Edit `.env` and set your configuration:
   ```env
   TELEGRAM_BOT_TOKEN=your_bot_token_from_botfather
   API_BASE_URL=http://localhost:8080
   WS_BASE_URL=ws://localhost:8080
   ```

## Getting a Telegram Bot Token

1. Open Telegram and search for [@BotFather](https://t.me/botfather)
2. Send `/newbot` command
3. Follow the instructions to create your bot
4. Copy the bot token and paste it in your `.env` file

## Usage

### Start the bot:
```bash
python bot.py
```

### Bot Commands:

- `/start` - Show welcome message and help
- `/help` - Show help information
- `/monitor <address>` - Start monitoring a Tron address
- `/stop_monitor <address>` - Stop monitoring a specific address
- `/stop_all` - Stop all active monitors
- `/list` - List all active monitors

### Examples:

**Monitor an address:**
```
/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

**Stop monitoring:**
```
/stop_monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf
```

**Stop all monitors:**
```
/stop_all
```

**List active monitors:**
```
/list
```

## What You'll See

When a transaction occurs on a monitored address, you'll receive a notification with:

- 📦 Block number
- 🔗 Transaction hash
- 📤 From address
- 📥 To address
- 💰 Amount (in TRX)
- 📋 Transaction type
- 🔍 Smart contract details (if applicable):
  - Method name
  - Parameter addresses
  - Token amounts
- ✅/❌ Transaction status

## Running with API Server

Make sure your Mongotron API server is running before starting the bot:

```bash
# In the main mongotron directory
./bin/api-server
```

The bot will connect to:
- REST API: `http://localhost:8080/api`
- WebSocket: `ws://localhost:8080/ws/events`

## Troubleshooting

### "Failed to register address for monitoring"
- Ensure the API server is running
- Check that `API_BASE_URL` in `.env` is correct
- Verify the address format is valid (starts with 'T', 34 characters)

### "Connection lost for address"
- The WebSocket connection was interrupted
- Check your network connection
- Verify the API server is still running
- The bot will automatically clean up the monitor

### "Import errors" when running
- Make sure you've activated the virtual environment
- Run `pip install -r requirements.txt` again

## Features in Detail

### Multi-Address Monitoring
You can monitor multiple addresses simultaneously. Each address gets its own WebSocket connection and sends independent notifications.

### Smart Contract Decoding
When a smart contract interaction is detected, the bot shows:
- Function name (e.g., `transfer(address,uint256)`)
- Parameter addresses extracted from the call
- Token amounts involved

### Address Formatting
Long addresses are automatically truncated for better readability in notifications while full addresses are shown in code blocks for copying.

## Development

The bot uses:
- **aiogram 3.x** - Modern async Telegram bot framework
- **aiohttp** - Async HTTP client for API calls
- **websockets** - WebSocket client for real-time events
- **python-dotenv** - Environment variable management

## Architecture

```
┌─────────────┐      REST API      ┌──────────────┐
│             │◄──────────────────►│              │
│ Telegram    │                    │  Mongotron   │
│    Bot      │      WebSocket     │  API Server  │
│             │◄──────────────────►│              │
└─────────────┘                    └──────────────┘
      │                                    │
      │                                    │
      ▼                                    ▼
┌─────────────┐                    ┌──────────────┐
│  Telegram   │                    │   MongoDB    │
│   Users     │                    │   Database   │
└─────────────┘                    └──────────────┘
```

## Security Notes

- Never commit your `.env` file or bot token
- The `.env` file is git-ignored by default
- Keep your bot token secure
- Consider rate limiting for production use

## License

Part of the Mongotron project.
