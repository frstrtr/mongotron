# 📚 Telegram Bot - Documentation Index

Welcome to the Mongotron Telegram Bot documentation!

## 🎯 Start Here

**New to the bot? Start with one of these:**

1. **[QUICKREF.txt](QUICKREF.txt)** ⚡
   - One-page quick reference
   - Commands at a glance
   - Perfect for quick lookups

2. **[SETUP.md](SETUP.md)** 🚀
   - Complete setup guide
   - Step-by-step instructions
   - Troubleshooting tips

3. **[README.md](README.md)** 📖
   - Project overview
   - Features and architecture
   - Installation guide

## 📖 Full Documentation

### For Users

| Document | Purpose | When to Use |
|----------|---------|-------------|
| **[QUICKREF.txt](QUICKREF.txt)** | Quick reference card | Need a quick command reminder |
| **[SETUP.md](SETUP.md)** | Setup instructions | First-time setup |
| **[COMMANDS.md](COMMANDS.md)** | Command reference | Learn about specific commands |
| **[README.md](README.md)** | Project overview | Understand the project |

### For Developers

| Document | Purpose | Content |
|----------|---------|---------|
| **[bot.py](bot.py)** | Main application | 458 lines of bot code |
| **[test_bot.py](test_bot.py)** | Testing script | Configuration verification |
| **[PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)** | Project details | Statistics and architecture |

### Configuration Files

| File | Purpose |
|------|---------|
| **[requirements.txt](requirements.txt)** | Python dependencies |
| **[.env.example](.env.example)** | Configuration template |
| **[.gitignore](.gitignore)** | Git ignore rules |
| **[mongotron-bot.service](mongotron-bot.service)** | Systemd service |
| **[start.sh](start.sh)** | Quick start script |

## 🗺️ Documentation Roadmap

### 1️⃣ First Time? → Start Here
```
QUICKREF.txt → SETUP.md → Try the bot!
```

### 2️⃣ Need Details? → Deep Dive
```
README.md → COMMANDS.md → bot.py
```

### 3️⃣ Deploying? → Production Guide
```
SETUP.md (Service section) → mongotron-bot.service
```

### 4️⃣ Developing? → Code Dive
```
PROJECT_SUMMARY.md → bot.py → test_bot.py
```

## 📋 Quick Navigation

### Common Tasks

**Set up the bot:**
→ [SETUP.md](SETUP.md) - Complete setup guide

**Learn commands:**
→ [COMMANDS.md](COMMANDS.md) - All commands explained

**Quick reference:**
→ [QUICKREF.txt](QUICKREF.txt) - One-page cheat sheet

**Run as service:**
→ [SETUP.md](SETUP.md#running-as-service) - Systemd setup

**Troubleshooting:**
→ [SETUP.md](SETUP.md#troubleshooting) - Common issues

**Architecture:**
→ [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md#architecture) - System design

## 📊 Document Sizes

| Document | Size | Lines |
|----------|------|-------|
| bot.py | 13K | 458 |
| test_bot.py | 3.6K | 115 |
| README.md | 5.3K | 268 |
| COMMANDS.md | 7.5K | 321 |
| SETUP.md | 9.6K | 391 |
| PROJECT_SUMMARY.md | 9.4K | 366 |
| QUICKREF.txt | 11K | 183 |
| **TOTAL** | **~60K** | **~2,100** |

## 🎓 Learning Path

### Beginner
1. Read [QUICKREF.txt](QUICKREF.txt) (5 min)
2. Follow [SETUP.md](SETUP.md) (15 min)
3. Try `/start` command in bot
4. Monitor your first address!

### Intermediate
1. Read [README.md](README.md) (10 min)
2. Study [COMMANDS.md](COMMANDS.md) (20 min)
3. Try all commands
4. Monitor multiple addresses

### Advanced
1. Read [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md) (15 min)
2. Study [bot.py](bot.py) (30 min)
3. Run [test_bot.py](test_bot.py)
4. Deploy as systemd service

## 🔍 Find Information Fast

### "How do I...?"

**...install the bot?**
→ [SETUP.md](SETUP.md#installation)

**...get a bot token?**
→ [SETUP.md](SETUP.md#getting-a-telegram-bot-token)

**...monitor an address?**
→ [COMMANDS.md](COMMANDS.md#monitor-address)

**...stop monitoring?**
→ [COMMANDS.md](COMMANDS.md#stop_monitor-address)

**...run as a service?**
→ [SETUP.md](SETUP.md#running-as-service)

**...troubleshoot issues?**
→ [SETUP.md](SETUP.md#troubleshooting)

**...understand notifications?**
→ [COMMANDS.md](COMMANDS.md#transaction-details-explained)

**...customize the bot?**
→ [bot.py](bot.py) + [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md#customization)

## 🎯 By Role

### End User
You want to monitor addresses:
1. [QUICKREF.txt](QUICKREF.txt) - Commands
2. [SETUP.md](SETUP.md) - Setup
3. [COMMANDS.md](COMMANDS.md) - Usage

### System Administrator  
You want to deploy the bot:
1. [SETUP.md](SETUP.md) - Installation
2. [mongotron-bot.service](mongotron-bot.service) - Service
3. [test_bot.py](test_bot.py) - Testing

### Developer
You want to understand/modify code:
1. [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md) - Overview
2. [bot.py](bot.py) - Source code
3. [README.md](README.md) - Architecture

## 📱 Bot Commands Quick Reference

```
/start              Show welcome message
/monitor <addr>     Start monitoring
/stop_monitor <a>   Stop monitoring address
/stop_all           Stop all monitors
/list               List active monitors
/help               Show help
```

Full details: [COMMANDS.md](COMMANDS.md)

## 🔧 Technical Stack

- **Python 3.8+**
- **aiogram 3.13.1** - Telegram bot framework
- **aiohttp 3.10.5** - HTTP client
- **websockets 13.1** - WebSocket client
- **python-dotenv 1.0.0** - Config management

Details: [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md#technology-stack)

## 🏗️ Architecture Overview

```
Telegram Users
     ↓
Telegram Bot (aiogram)
     ↓
Mongotron API Server
     ↓
MongoDB Database
```

Full diagram: [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md#architecture)

## ✅ Features at a Glance

✅ Real-time monitoring
✅ Multi-address support
✅ Smart contract decoding
✅ Token transfers
✅ Rich notifications
✅ Easy commands
✅ Auto-reconnect
✅ Production-ready

Full list: [README.md](README.md#features)

## 🚀 Quick Start (3 Steps)

```bash
# 1. Get bot token from @BotFather
# 2. Configure
cd examples/telegram-bot
cp .env.example .env
nano .env  # Add token

# 3. Start
./start.sh
```

Details: [SETUP.md](SETUP.md#quick-start-3-steps)

## 📞 Support

### Documentation
All questions should be answerable from:
- [QUICKREF.txt](QUICKREF.txt) - Quick answers
- [SETUP.md](SETUP.md) - Setup & troubleshooting
- [COMMANDS.md](COMMANDS.md) - Command usage

### Testing
Run configuration test:
```bash
python test_bot.py
```

Details: [test_bot.py](test_bot.py)

## 🔄 Updates

### Check bot.py
Current version implements:
- 6 commands
- WebSocket monitoring
- Smart contract decoding
- Multi-address support

See: [bot.py](bot.py) and [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md)

## 🎁 What's Included

### Core Files
- ✅ Bot application (bot.py)
- ✅ Test script (test_bot.py)
- ✅ Setup script (start.sh)

### Configuration
- ✅ Dependencies (requirements.txt)
- ✅ Environment template (.env.example)
- ✅ Git rules (.gitignore)
- ✅ Service file (mongotron-bot.service)

### Documentation
- ✅ Quick reference (QUICKREF.txt)
- ✅ Setup guide (SETUP.md)
- ✅ Command reference (COMMANDS.md)
- ✅ README (README.md)
- ✅ Project summary (PROJECT_SUMMARY.md)
- ✅ This index (INDEX.md)

**Everything you need!** 🎉

## 🌟 Get Started Now!

1. **Quick Look**: [QUICKREF.txt](QUICKREF.txt) (2 minutes)
2. **Setup**: [SETUP.md](SETUP.md) (10 minutes)
3. **Start Monitoring**: Open Telegram! (30 seconds)

## 📚 Document Tree

```
telegram-bot/
├── INDEX.md                    ← You are here
├── QUICKREF.txt               ← Start here for quick ref
├── SETUP.md                   ← Start here for setup
├── README.md                  ← Project overview
├── COMMANDS.md                ← Command details
├── PROJECT_SUMMARY.md         ← Technical details
├── bot.py                     ← Main code
├── test_bot.py               ← Testing
├── start.sh                  ← Setup script
├── requirements.txt          ← Dependencies
├── .env.example              ← Config template
├── .gitignore               ← Git rules
└── mongotron-bot.service    ← Service file
```

---

**Need help?** Check the relevant documentation above!

**Ready to start?** Run `./start.sh` and follow the prompts!

**Questions?** Review [SETUP.md](SETUP.md#troubleshooting) troubleshooting section!

Happy monitoring! 🚀
