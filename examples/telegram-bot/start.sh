#!/bin/bash

# Quick start script for Telegram Bot

echo "=========================================="
echo "Mongotron Telegram Bot - Quick Start"
echo "=========================================="
echo ""

# Check if we're in the telegram-bot directory
if [ ! -f "bot.py" ]; then
    echo "Error: Please run this script from the telegram-bot directory"
    exit 1
fi

# Check if .env exists
if [ ! -f ".env" ]; then
    echo "⚠️  .env file not found!"
    echo ""
    echo "Creating .env from .env.example..."
    cp .env.example .env
    echo ""
    echo "✅ .env file created!"
    echo ""
    echo "⚠️  IMPORTANT: Please edit .env and set your TELEGRAM_BOT_TOKEN"
    echo "   Get your token from @BotFather on Telegram"
    echo ""
    read -p "Press Enter after you've configured .env..."
fi

# Check if virtual environment exists
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
    echo "✅ Virtual environment created"
    echo ""
fi

# Activate virtual environment
echo "Activating virtual environment..."
source venv/bin/activate

# Install dependencies
echo "Installing dependencies..."
pip install -q -r requirements.txt
echo "✅ Dependencies installed"
echo ""

# Run configuration test
echo "Running configuration test..."
echo ""
python test_bot.py
echo ""

# Ask if user wants to start the bot
read -p "Start the bot now? (y/n): " answer
if [ "$answer" = "y" ]; then
    echo ""
    echo "Starting bot..."
    echo "Press Ctrl+C to stop"
    echo ""
    python bot.py
else
    echo ""
    echo "To start the bot later, run:"
    echo "  source venv/bin/activate"
    echo "  python bot.py"
fi
