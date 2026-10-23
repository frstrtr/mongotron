#!/bin/bash
# MongoTron Quick Start Script
# Usage: ./start.sh

set -e

echo "╔══════════════════════════════════════════════════════════════════════════════╗"
echo "║                                                                              ║"
echo "║                     🚀 MongoTron Quick Start                                 ║"
echo "║                                                                              ║"
echo "╚══════════════════════════════════════════════════════════════════════════════╝"
echo ""

# Check if we're in the right directory
if [ ! -f "event_monitor.py" ]; then
    echo "❌ Error: Please run this script from the mongotron directory"
    exit 1
fi

# Check if API server is running
echo "🔍 Checking API server status..."
if curl -s http://localhost:8080/api/v1/subscriptions > /dev/null 2>&1; then
    echo "✅ API server is running on http://localhost:8080"
else
    echo "⚠️  API server not running. Starting it now..."
    nohup ./bin/api-server > /tmp/api-server.log 2>&1 &
    sleep 3
    if curl -s http://localhost:8080/api/v1/subscriptions > /dev/null 2>&1; then
        echo "✅ API server started successfully!"
    else
        echo "❌ Failed to start API server. Check /tmp/api-server.log for details."
        exit 1
    fi
fi

echo ""
echo "📊 System Status:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Get subscription count
ACTIVE_SUBS=$(curl -s http://localhost:8080/api/v1/subscriptions | jq -r '[.subscriptions[] | select(.status=="active")] | length')
TOTAL_EVENTS=$(curl -s http://localhost:8080/api/v1/events | jq -r '.total')

echo "   🟢 MongoDB:         nileVM.lan:27017"
echo "   🟢 Tron Node:       nileVM.lan:50051 (Nile Testnet)"
echo "   📈 Active Subs:     $ACTIVE_SUBS subscriptions"
echo "   📊 Events Captured: $TOTAL_EVENTS events"
echo ""

echo "🎯 Ready to monitor events!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "To start monitoring, run:"
echo ""
echo "   source .venv/bin/activate"
echo "   python event_monitor.py"
echo ""
echo "Or check the status with:"
echo ""
echo "   curl http://localhost:8080/api/v1/subscriptions | jq ."
echo ""
echo "📚 Documentation: See QUICK_START.md, STARTUP_GUIDE.md, or HOW_TO_START.md"
echo ""
