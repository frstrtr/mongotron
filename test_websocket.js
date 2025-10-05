#!/usr/bin/env node
// MongoTron WebSocket Client Test
// Usage: node test_websocket.js <subscription_id>

const WebSocket = require('ws');

const subscriptionId = process.argv[2];
if (!subscriptionId) {
  console.error('Usage: node test_websocket.js <subscription_id>');
  process.exit(1);
}

const wsUrl = `ws://localhost:8080/api/v1/events/stream/${subscriptionId}`;
console.log(`Connecting to: ${wsUrl}`);
console.log('');

const ws = new WebSocket(wsUrl);

ws.on('open', () => {
  console.log('✅ Connected to MongoTron event stream');
  console.log(`📡 Subscription ID: ${subscriptionId}`);
  console.log('🔄 Waiting for events...');
  console.log('');
});

ws.on('message', (data) => {
  try {
    const event = JSON.parse(data);
    
    if (event.type === 'connected') {
      console.log('📨 Welcome Message:');
      console.log(JSON.stringify(event, null, 2));
      console.log('');
      return;
    }
    
    console.log('🔔 Event Received:');
    console.log('─────────────────────────────────────────');
    console.log(`  Event ID:     ${event.eventId || 'N/A'}`);
    console.log(`  Network:      ${event.network || 'N/A'}`);
    console.log(`  Type:         ${event.type || 'N/A'}`);
    console.log(`  TX Hash:      ${event.txHash || 'N/A'}`);
    console.log(`  Block:        ${event.blockNumber || 'N/A'}`);
    console.log(`  Timestamp:    ${new Date((event.blockTimestamp || 0) * 1000).toISOString()}`);
    
    if (event.data) {
      console.log('  Data:');
      console.log(`    From:       ${event.data.from || 'N/A'}`);
      console.log(`    To:         ${event.data.to || 'N/A'}`);
      console.log(`    Amount:     ${event.data.amount || 0} SUN`);
      console.log(`    Asset:      ${event.data.asset || 'N/A'}`);
      console.log(`    Success:    ${event.data.success ? '✅' : '❌'}`);
    }
    
    console.log('─────────────────────────────────────────');
    console.log('');
  } catch (err) {
    console.error('❌ Error parsing message:', err.message);
    console.log('Raw data:', data.toString());
  }
});

ws.on('error', (error) => {
  console.error('❌ WebSocket Error:', error.message);
});

ws.on('close', (code, reason) => {
  console.log('');
  console.log('🔌 Disconnected from event stream');
  console.log(`   Code: ${code}`);
  console.log(`   Reason: ${reason || 'No reason provided'}`);
});

// Handle Ctrl+C gracefully
process.on('SIGINT', () => {
  console.log('');
  console.log('🛑 Closing connection...');
  ws.close();
  process.exit(0);
});

// Send ping to keep connection alive (optional)
setInterval(() => {
  if (ws.readyState === WebSocket.OPEN) {
    ws.ping();
  }
}, 30000); // Every 30 seconds
