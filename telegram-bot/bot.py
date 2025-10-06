"""
Telegram Bot for Mongotron - Monitor blockchain addresses in real-time
"""
import asyncio
import logging
import os
from typing import Dict, Set
from datetime import datetime
import base58
import hashlib

from aiogram import Bot, Dispatcher, types, F
from aiogram.filters import Command
from aiogram.types import Message
from dotenv import load_dotenv
import aiohttp
import websockets
import json

# Load environment variables
load_dotenv()

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Bot configuration
TELEGRAM_BOT_TOKEN = os.getenv('TELEGRAM_BOT_TOKEN')
API_BASE_URL = os.getenv('API_BASE_URL', 'http://localhost:8080')
WS_BASE_URL = os.getenv('WS_BASE_URL', 'ws://localhost:8080')

if not TELEGRAM_BOT_TOKEN:
    raise ValueError("TELEGRAM_BOT_TOKEN not found in environment variables")

# Initialize bot and dispatcher
bot = Bot(token=TELEGRAM_BOT_TOKEN)
dp = Dispatcher()

# Store active monitors: {chat_id: {address: task}}
active_monitors: Dict[int, Dict[str, asyncio.Task]] = {}


def hex_to_base58(hex_address: str) -> str:
    """Convert hex address (41...) to Base58 format (T...)"""
    try:
        if not hex_address:
            return hex_address
        
        # Remove '41' prefix if present and convert to bytes
        if hex_address.startswith('41'):
            address_bytes = bytes.fromhex(hex_address)
        else:
            # Assume it's already in correct format
            return hex_address
        
        # Calculate checksum
        hash0 = hashlib.sha256(address_bytes).digest()
        hash1 = hashlib.sha256(hash0).digest()
        checksum = hash1[:4]
        
        # Encode with checksum
        address_with_checksum = address_bytes + checksum
        base58_address = base58.b58encode(address_with_checksum).decode('utf-8')
        
        return base58_address
    except Exception as e:
        logger.warning(f"Failed to convert address {hex_address}: {e}")
        return hex_address


def format_address(address: str) -> str:
    """Format address for display (convert to Base58 and truncate middle)"""
    # Convert hex to Base58 if needed
    if address.startswith('41'):
        address = hex_to_base58(address)
    
    if len(address) > 20:
        return f"{address[:10]}...{address[-10:]}"
    return address


def format_tx_link(tx_hash: str) -> str:
    """Format transaction hash as a clickable TronScan link"""
    if not tx_hash:
        return "N/A"
    
    short_hash = f"{tx_hash[:10]}...{tx_hash[-10:]}" if len(tx_hash) > 20 else tx_hash
    # Use Nile testnet TronScan
    return f'<a href="https://nile.tronscan.org/#/transaction/{tx_hash}">{short_hash}</a>'


def format_address_link(address: str) -> str:
    """Format address as a clickable TronScan link (converts hex to Base58 first)"""
    if not address:
        return "N/A"
    
    # Convert hex to Base58 if needed
    if address.startswith('41'):
        address = hex_to_base58(address)
    
    short_addr = f"{address[:10]}...{address[-10:]}" if len(address) > 20 else address
    # Use Nile testnet TronScan
    return f'<a href="https://nile.tronscan.org/#/address/{address}">{short_addr}</a>'


def format_event(event: dict) -> str:
    """Format event data for Telegram message"""
    # Skip empty or invalid events
    if not event or not isinstance(event, dict):
        return None
    
    # Check if event has any meaningful data (skip connection messages)
    # Note: API uses capital letters for field names (BlockNumber, TransactionID, etc.)
    has_data = any(key in event for key in ['BlockNumber', 'blockNumber', 'TransactionID', 'TransactionHash', 'txHash', 'From', 'from', 'To', 'to', 'ContractType', 'contractType'])
    if not has_data:
        logger.warning(f"Event has no meaningful data: {event}")
        return None
    
    msg_lines = []
    
    # Header
    msg_lines.append("🔔 <b>New Transaction Detected</b>")
    msg_lines.append("─" * 40)
    
    # Block info (try both uppercase and lowercase)
    block_number = event.get('BlockNumber') or event.get('blockNumber')
    if block_number:
        msg_lines.append(f"📦 Block: <code>{block_number}</code>")
    
    # Transaction hash (try multiple field names) - make it a clickable link
    tx_hash = event.get('TransactionID') or event.get('TransactionHash') or event.get('txHash')
    if tx_hash:
        msg_lines.append(f"🔗 TX: {format_tx_link(tx_hash)}")
    
    # From/To addresses (convert hex to Base58 and make clickable)
    from_addr = event.get('From') or event.get('from')
    if from_addr:
        msg_lines.append(f"📤 From: {format_address_link(from_addr)}")
    
    to_addr = event.get('To') or event.get('to')
    if to_addr:
        msg_lines.append(f"📥 To: {format_address_link(to_addr)}")
    
    # Amount
    amount = event.get('Amount') or event.get('amount')
    if amount and amount > 0:
        amount_trx = amount / 1_000_000  # Convert from SUN to TRX
        msg_lines.append(f"💰 Amount: <b>{amount_trx:.6f} TRX</b>")
    
    # Contract type
    contract_type = event.get('ContractType') or event.get('contractType')
    if contract_type:
        msg_lines.append(f"📋 Type: <code>{contract_type}</code>")
    
    # Smart contract decoded info (check both EventData and direct smartContract)
    event_data = event.get('EventData', {})
    sc = event_data.get('smartContract') or event.get('smartContract', {})
    if sc:
        msg_lines.append("")
        msg_lines.append("🔍 <b>Smart Contract Details:</b>")
        
        method_name = sc.get('methodName')
        if method_name:
            msg_lines.append(f"   ⚙️ Method: <code>{method_name}</code>")
        
        addresses = sc.get('addresses', [])
        if addresses:
            msg_lines.append(f"   📍 Param Addresses:")
            for addr in addresses[:3]:  # Limit to 3 addresses
                msg_lines.append(f"      • {format_address_link(addr)}")
        
        sc_amount = sc.get('amount')
        if sc_amount:
            msg_lines.append(f"   💵 Token Amount: <code>{sc_amount}</code>")
        
        # Show decoded parameters if available
        params = sc.get('parameters', {})
        if params:
            msg_lines.append(f"   📝 Parameters:")
            for key, value in list(params.items())[:3]:  # Limit to 3 params
                if isinstance(value, str):
                    # Check if it's an address (starts with 41 or T)
                    if value.startswith('41') and len(value) == 42:
                        formatted_value = format_address_link(value)
                        msg_lines.append(f"      • {key}: {formatted_value}")
                    elif value.startswith('T') and len(value) == 34:
                        formatted_value = format_address_link(value)
                        msg_lines.append(f"      • {key}: {formatted_value}")
                    elif len(value) < 50:
                        msg_lines.append(f"      • {key}: <code>{value}</code>")
    
    # Status
    success = event.get('Success')
    if success is None:
        success = event.get('success')
    if success is not None:
        status = "✅ Success" if success else "❌ Failed"
        msg_lines.append(f"\n{status}")
    
    # Timestamp
    timestamp = datetime.now().strftime('%H:%M:%S')
    msg_lines.append(f"\n⏰ {timestamp}")
    
    return "\n".join(msg_lines)


async def monitor_address(chat_id: int, address: str):
    """Monitor an address via WebSocket and send updates to chat"""
    subscription_id = None
    try:
        # Create subscription for address monitoring via API
        async with aiohttp.ClientSession() as session:
            async with session.post(
                f"{API_BASE_URL}/api/v1/subscriptions",
                json={"address": address}
            ) as resp:
                if resp.status != 200 and resp.status != 201:
                    error_text = await resp.text()
                    logger.error(f"Failed to create subscription for {address}: {error_text}")
                    await bot.send_message(
                        chat_id,
                        f"❌ Failed to create subscription for monitoring: {error_text}"
                    )
                    return
                
                # Get subscription ID from response
                result = await resp.json()
                subscription_id = result.get('subscriptionId') or result.get('id')
                if not subscription_id:
                    logger.error(f"No subscription ID in response: {result}")
                    await bot.send_message(
                        chat_id,
                        f"❌ Failed to get subscription ID from server"
                    )
                    return
                
                logger.info(f"Created subscription {subscription_id} for address {address}")
        
        # Connect to WebSocket with subscription ID
        ws_url = f"{WS_BASE_URL}/api/v1/events/stream/{subscription_id}"
        logger.info(f"Connecting to WebSocket: {ws_url}")
        
        async with websockets.connect(ws_url) as websocket:
            await bot.send_message(
                chat_id,
                f"✅ Now monitoring address:\n<code>{address}</code>\n\n"
                f"You'll receive notifications for all transactions related to this address.",
                parse_mode="HTML"
            )
            
            # Listen for events
            async for message in websocket:
                try:
                    # Log raw message for debugging
                    logger.debug(f"Received WebSocket message: {message[:200]}...")
                    
                    event = json.loads(message)
                    logger.info(f"Parsed event: {event}")
                    
                    # Format and send the event
                    formatted_msg = format_event(event)
                    
                    # Only send if we have a valid formatted message
                    if formatted_msg:
                        await bot.send_message(
                            chat_id,
                            formatted_msg,
                            parse_mode="HTML",
                            disable_web_page_preview=True
                        )
                    else:
                        logger.warning(f"Skipped empty or invalid event: {event}")
                    
                except json.JSONDecodeError as e:
                    logger.error(f"Failed to decode WebSocket message: {message}")
                    logger.error(f"JSON decode error: {e}")
                except Exception as e:
                    logger.error(f"Error processing event: {e}", exc_info=True)
                    
    except websockets.exceptions.WebSocketException as e:
        logger.error(f"WebSocket error for {address}: {e}")
        await bot.send_message(
            chat_id,
            f"❌ Connection lost for address:\n<code>{address}</code>\n\n"
            f"Error: {str(e)}",
            parse_mode="HTML"
        )
    except Exception as e:
        logger.error(f"Error monitoring {address}: {e}")
        await bot.send_message(
            chat_id,
            f"❌ Error monitoring address:\n<code>{address}</code>\n\n"
            f"Error: {str(e)}",
            parse_mode="HTML"
        )
    finally:
        # Clean up
        if chat_id in active_monitors and address in active_monitors[chat_id]:
            del active_monitors[chat_id][address]
            if not active_monitors[chat_id]:
                del active_monitors[chat_id]


@dp.message(Command("start"))
async def cmd_start(message: Message):
    """Handle /start command"""
    welcome_text = """
👋 <b>Welcome to Mongotron Monitor Bot!</b>

This bot helps you monitor Tron blockchain addresses in real-time on <b>Nile Testnet</b>.

<b>Available commands:</b>

/monitor &lt;address&gt; - Start monitoring an address
/stop_monitor &lt;address&gt; - Stop monitoring an address
/stop_all - Stop all active monitors
/list - List all active monitors
/help - Show this help message

<b>Examples:</b>
<code>/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf</code>
<code>/monitor TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf</code> (Nile USDT)

You'll receive real-time notifications for:
• Incoming transactions
• Outgoing transactions
• Smart contract interactions
• Token transfers

<b>Network:</b> Tron Nile Testnet
"""
    await message.answer(welcome_text, parse_mode="HTML")


@dp.message(Command("help"))
async def cmd_help(message: Message):
    """Handle /help command"""
    await cmd_start(message)


@dp.message(Command("monitor"))
async def cmd_monitor(message: Message):
    """Handle /monitor command"""
    # Extract address from command
    parts = message.text.split(maxsplit=1)
    if len(parts) < 2:
        await message.answer(
            "❌ Please provide an address to monitor.\n\n"
            "<b>Usage:</b> <code>/monitor &lt;address&gt;</code>\n\n"
            "<b>Examples:</b>\n"
            "<code>/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf</code>\n"
            "<code>/monitor TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf</code> (Nile USDT)\n\n"
            "<b>Network:</b> Tron Nile Testnet",
            parse_mode="HTML"
        )
        return
    
    address = parts[1].strip()
    chat_id = message.chat.id
    
    # Validate address format (basic check)
    if not address.startswith('T') or len(address) < 30:
        await message.answer(
            "❌ Invalid address format. Tron addresses should start with 'T' and be 34 characters long.\n\n"
            "<b>Example:</b> <code>TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf</code>",
            parse_mode="HTML"
        )
        return
    
    # Check if already monitoring
    if chat_id in active_monitors and address in active_monitors[chat_id]:
        await message.answer(
            f"ℹ️ Already monitoring address:\n<code>{address}</code>",
            parse_mode="HTML"
        )
        return
    
    # Start monitoring
    await message.answer(
        f"🔄 Starting monitor for address:\n<code>{address}</code>\n\n"
        f"Please wait...",
        parse_mode="HTML"
    )
    
    # Create monitoring task
    task = asyncio.create_task(monitor_address(chat_id, address))
    
    # Store the task
    if chat_id not in active_monitors:
        active_monitors[chat_id] = {}
    active_monitors[chat_id][address] = task
    
    logger.info(f"Started monitoring {address} for chat {chat_id}")


@dp.message(Command("stop_monitor"))
async def cmd_stop_monitor(message: Message):
    """Handle /stop_monitor command"""
    # Extract address from command
    parts = message.text.split(maxsplit=1)
    if len(parts) < 2:
        await message.answer(
            "❌ Please provide an address to stop monitoring.\n\n"
            "<b>Usage:</b> <code>/stop_monitor &lt;address&gt;</code>",
            parse_mode="HTML"
        )
        return
    
    address = parts[1].strip()
    chat_id = message.chat.id
    
    # Check if monitoring this address
    if chat_id not in active_monitors or address not in active_monitors[chat_id]:
        await message.answer(
            f"ℹ️ Not currently monitoring address:\n<code>{address}</code>",
            parse_mode="HTML"
        )
        return
    
    # Stop the monitoring task
    task = active_monitors[chat_id][address]
    task.cancel()
    
    try:
        await task
    except asyncio.CancelledError:
        pass
    
    # Remove from active monitors
    del active_monitors[chat_id][address]
    if not active_monitors[chat_id]:
        del active_monitors[chat_id]
    
    await message.answer(
        f"✅ Stopped monitoring address:\n<code>{address}</code>",
        parse_mode="HTML"
    )
    
    logger.info(f"Stopped monitoring {address} for chat {chat_id}")


@dp.message(Command("stop_all"))
async def cmd_stop_all(message: Message):
    """Handle /stop_all command"""
    chat_id = message.chat.id
    
    # Check if any monitors are active
    if chat_id not in active_monitors or not active_monitors[chat_id]:
        await message.answer("ℹ️ No active monitors to stop.")
        return
    
    # Stop all monitoring tasks
    addresses = list(active_monitors[chat_id].keys())
    for address in addresses:
        task = active_monitors[chat_id][address]
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass
    
    # Clear all monitors for this chat
    del active_monitors[chat_id]
    
    await message.answer(
        f"✅ Stopped all monitors ({len(addresses)} addresses):\n\n" +
        "\n".join([f"• <code>{format_address(addr)}</code>" for addr in addresses]),
        parse_mode="HTML"
    )
    
    logger.info(f"Stopped all monitors for chat {chat_id}")


@dp.message(Command("list"))
async def cmd_list(message: Message):
    """Handle /list command"""
    chat_id = message.chat.id
    
    # Check if any monitors are active
    if chat_id not in active_monitors or not active_monitors[chat_id]:
        await message.answer("ℹ️ No active monitors.")
        return
    
    # List all active monitors
    addresses = list(active_monitors[chat_id].keys())
    
    msg = f"📋 <b>Active Monitors ({len(addresses)}):</b>\n\n"
    for i, address in enumerate(addresses, 1):
        msg += f"{i}. <code>{address}</code>\n"
    
    msg += f"\n💡 Use <code>/stop_monitor &lt;address&gt;</code> to stop a specific monitor"
    msg += f"\n💡 Use <code>/stop_all</code> to stop all monitors"
    
    await message.answer(msg, parse_mode="HTML")


async def main():
    """Main function to run the bot"""
    logger.info("Starting Mongotron Telegram Bot...")
    logger.info(f"API Base URL: {API_BASE_URL}")
    logger.info(f"WebSocket Base URL: {WS_BASE_URL}")
    
    try:
        # Start polling
        await dp.start_polling(bot)
    finally:
        # Clean up all active monitors
        for chat_id in list(active_monitors.keys()):
            for address, task in list(active_monitors[chat_id].items()):
                task.cancel()
                try:
                    await task
                except asyncio.CancelledError:
                    pass
        
        await bot.session.close()


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Bot stopped by user")
    except Exception as e:
        logger.error(f"Fatal error: {e}")
