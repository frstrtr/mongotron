#!/bin/bash
# MongoTron API Server Test Script

set -e

API_URL="http://localhost:8080"
BASE_PATH="/api/v1"

echo "================================"
echo "MongoTron API Server Test Script"
echo "================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test 1: Health Check
echo -e "${BLUE}Test 1: Health Check${NC}"
curl -s "${API_URL}${BASE_PATH}/health" | jq '.'
echo ""
echo ""

# Test 2: Create Subscription
echo -e "${BLUE}Test 2: Create Subscription${NC}"
SUBSCRIPTION=$(curl -s -X POST "${API_URL}${BASE_PATH}/subscriptions" \
  -H "Content-Type: application/json" \
  -d '{
    "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
    "webhookUrl": "",
    "filters": {
      "contractTypes": [],
      "minAmount": 0,
      "maxAmount": 0,
      "onlySuccess": false
    },
    "startBlock": 0
  }')

echo "$SUBSCRIPTION" | jq '.'
SUBSCRIPTION_ID=$(echo "$SUBSCRIPTION" | jq -r '.subscriptionId')
echo ""
echo -e "${GREEN}Subscription ID: $SUBSCRIPTION_ID${NC}"
echo ""

# Test 3: List Subscriptions
echo -e "${BLUE}Test 3: List Subscriptions${NC}"
curl -s "${API_URL}${BASE_PATH}/subscriptions?limit=10&skip=0" | jq '.'
echo ""
echo ""

# Test 4: Get Specific Subscription
echo -e "${BLUE}Test 4: Get Subscription by ID${NC}"
curl -s "${API_URL}${BASE_PATH}/subscriptions/${SUBSCRIPTION_ID}" | jq '.'
echo ""
echo ""

# Test 5: List Events
echo -e "${BLUE}Test 5: List Events${NC}"
curl -s "${API_URL}${BASE_PATH}/events?limit=5" | jq '.'
echo ""
echo ""

# Test 6: Query Events by Address
echo -e "${BLUE}Test 6: Query Events by Address${NC}"
curl -s "${API_URL}${BASE_PATH}/events?address=TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf&limit=5" | jq '.'
echo ""
echo ""

# Test 7: Readiness Check
echo -e "${BLUE}Test 7: Readiness Check${NC}"
curl -s "${API_URL}${BASE_PATH}/ready" | jq '.'
echo ""
echo ""

# Test 8: Liveness Check
echo -e "${BLUE}Test 8: Liveness Check${NC}"
curl -s "${API_URL}${BASE_PATH}/live" | jq '.'
echo ""
echo ""

# Test 9: WebSocket Connection Info
echo -e "${BLUE}Test 9: WebSocket Connection${NC}"
echo "To connect to WebSocket:"
echo "ws://localhost:8080${BASE_PATH}/events/stream/${SUBSCRIPTION_ID}"
echo ""
echo "Example (Node.js):"
echo "const ws = new WebSocket('ws://localhost:8080${BASE_PATH}/events/stream/${SUBSCRIPTION_ID}');"
echo "ws.on('message', data => console.log(JSON.parse(data)));"
echo ""
echo ""

# Wait for user input before cleanup
echo -e "${BLUE}Press Enter to delete the subscription and exit...${NC}"
read

# Test 10: Delete Subscription
echo -e "${BLUE}Test 10: Delete Subscription${NC}"
curl -s -X DELETE "${API_URL}${BASE_PATH}/subscriptions/${SUBSCRIPTION_ID}" | jq '.'
echo ""
echo ""

echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}All tests completed successfully!${NC}"
echo -e "${GREEN}================================${NC}"
