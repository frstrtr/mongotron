"""
Test script to verify bot configuration and API connectivity
"""
import os
import asyncio
import aiohttp
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

API_BASE_URL = os.getenv('API_BASE_URL', 'http://localhost:8080')
TELEGRAM_BOT_TOKEN = os.getenv('TELEGRAM_BOT_TOKEN')


async def test_api_connection():
    """Test connection to Mongotron API"""
    print("Testing API connection...")
    print(f"API Base URL: {API_BASE_URL}")
    
    try:
        async with aiohttp.ClientSession() as session:
            # Test health endpoint
            async with session.get(f"{API_BASE_URL}/api/v1/health") as resp:
                if resp.status == 200:
                    print("✅ API server is reachable")
                    data = await resp.json()
                    print(f"   Response: {data}")
                else:
                    print(f"❌ API server returned status {resp.status}")
                    
    except aiohttp.ClientError as e:
        print(f"❌ Failed to connect to API: {e}")
    except Exception as e:
        print(f"❌ Error: {e}")


async def test_telegram_token():
    """Test Telegram bot token"""
    print("\nTesting Telegram bot token...")
    
    if not TELEGRAM_BOT_TOKEN:
        print("❌ TELEGRAM_BOT_TOKEN not set in .env file")
        return
    
    print(f"Token: {TELEGRAM_BOT_TOKEN[:10]}...{TELEGRAM_BOT_TOKEN[-10:]}")
    
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(
                f"https://api.telegram.org/bot{TELEGRAM_BOT_TOKEN}/getMe"
            ) as resp:
                if resp.status == 200:
                    data = await resp.json()
                    if data.get('ok'):
                        bot_info = data.get('result', {})
                        print("✅ Bot token is valid")
                        print(f"   Bot name: @{bot_info.get('username')}")
                        print(f"   Bot ID: {bot_info.get('id')}")
                    else:
                        print("❌ Invalid bot token")
                else:
                    print(f"❌ Failed to validate token (status {resp.status})")
                    
    except Exception as e:
        print(f"❌ Error validating token: {e}")


async def test_address_registration():
    """Test subscription creation with API"""
    print("\nTesting subscription creation...")
    
    test_address = "TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf"
    
    try:
        async with aiohttp.ClientSession() as session:
            async with session.post(
                f"{API_BASE_URL}/api/v1/subscriptions",
                json={"address": test_address}
            ) as resp:
                if resp.status in [200, 201]:
                    print(f"✅ Successfully created subscription for: {test_address}")
                    data = await resp.json()
                    print(f"   Response: {data}")
                else:
                    print(f"⚠️  Subscription creation returned status {resp.status}")
                    text = await resp.text()
                    print(f"   Response: {text}")
                    
    except Exception as e:
        print(f"❌ Error creating subscription: {e}")


async def main():
    """Run all tests"""
    print("=" * 60)
    print("Mongotron Telegram Bot - Configuration Test")
    print("=" * 60)
    
    await test_api_connection()
    await test_telegram_token()
    await test_address_registration()
    
    print("\n" + "=" * 60)
    print("Test completed!")
    print("=" * 60)


if __name__ == "__main__":
    asyncio.run(main())
