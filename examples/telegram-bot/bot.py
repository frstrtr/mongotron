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
    """Format address for display (truncate middle)"""
    if len(address) > 10:
        return f"{address[:6]}...{address[-4:]}"
    return address


def format_contract_type(contract_type: str) -> str:
    """Format contract type to be more human-readable with emoji"""
    type_mapping = {
        # Basic transfers
        'TransferContract': '💸 TRX Transfer',
        'TransferAssetContract': '🪙 TRC10 Token Transfer',
        
        # Smart contracts
        'TriggerSmartContract': '⚙️ Smart Contract Call',
        'CreateSmartContract': '📝 Create Smart Contract',
        'UpdateSettingContract': '⚙️ Update Contract Settings',
        'UpdateEnergyLimitContract': '⚡ Update Energy Limit',
        
        # Resource management
        'FreezeBalanceContract': '❄️ Freeze Balance',
        'UnfreezeBalanceContract': '🔥 Unfreeze Balance',
        'FreezeBalanceV2Contract': '❄️ Freeze Balance (v2)',
        'UnfreezeBalanceV2Contract': '🔥 Unfreeze Balance (v2)',
        'WithdrawExpireUnfreezeContract': '💰 Withdraw Unfrozen',
        'DelegateResourceContract': '🤝 Delegate Resources',
        'UnDelegateResourceContract': '↩️ Undelegate Resources',
        
        # Staking & Rewards
        'WithdrawBalanceContract': '💵 Withdraw Rewards',
        'VoteWitnessContract': '🗳️ Vote for Witness',
        
        # Account operations
        'AccountCreateContract': '👤 Create Account',
        'AccountUpdateContract': '✏️ Update Account',
        'SetAccountIdContract': '🆔 Set Account ID',
        'AccountPermissionUpdateContract': '🔐 Update Permissions',
        
        # Witness operations
        'WitnessCreateContract': '🏛️ Create Witness',
        'WitnessUpdateContract': '🏛️ Update Witness',
        'UpdateBrokerageContract': '💼 Update Brokerage',
        
        # Asset operations
        'AssetIssueContract': '🎫 Issue Asset',
        'UpdateAssetContract': '🎫 Update Asset',
        'ParticipateAssetIssueContract': '🛒 Buy Asset',
        'UnfreezeAssetContract': '🔓 Unfreeze Asset',
        
        # Proposals
        'ProposalCreateContract': '📋 Create Proposal',
        'ProposalApproveContract': '✅ Approve Proposal',
        'ProposalDeleteContract': '🗑️ Delete Proposal',
        
        # Exchange
        'ExchangeCreateContract': '🔄 Create Exchange',
        'ExchangeInjectContract': '💉 Inject to Exchange',
        'ExchangeWithdrawContract': '💰 Withdraw from Exchange',
        'ExchangeTransactionContract': '🔁 Exchange Transaction',
        
        # Market (TRX/TRC10)
        'MarketSellAssetContract': '📉 Market Sell',
        'MarketCancelOrderContract': '❌ Cancel Market Order',
        
        # Shield (Privacy)
        'ShieldedTransferContract': '🛡️ Shielded Transfer',
    }
    
    return type_mapping.get(contract_type, f'📄 {contract_type}')


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


def format_block_link(block_number: int) -> str:
    """Format block number as a clickable TronScan link"""
    if not block_number:
        return "N/A"
    
    # Use Nile testnet TronScan
    return f'<a href="https://nile.tronscan.org/#/block/{block_number}">{block_number}</a>'


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
    
    # Get event data early as we'll need it in multiple places
    event_data = event.get('EventData', {})
    
    msg_lines = []
    
    # Header
    msg_lines.append("🔔 <b>New Transaction Detected</b>")
    msg_lines.append("─" * 40)
    
    # Block info (try both uppercase and lowercase) - make it a clickable link
    block_number = event.get('BlockNumber') or event.get('blockNumber')
    if block_number:
        msg_lines.append(f"📦 Block: {format_block_link(block_number)}")
    
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
    
    # Contract type - use human-readable format with emoji
    contract_type = event.get('ContractType') or event.get('contractType')
    if contract_type:
        formatted_type = format_contract_type(contract_type)
        msg_lines.append(f"📋 Type: <b>{formatted_type}</b>")
    
    # Show additional details for specific transaction types
    if contract_type in ['DelegateResourceContract', 'UnDelegateResourceContract']:
        msg_lines.append("")
        # These are resource delegation operations
        resource = event_data.get('resource', 'BANDWIDTH')
        balance = event_data.get('balance', 0)
        if balance > 0:
            trx_amount = balance / 1_000_000
            msg_lines.append(f"   💎 Resource: <b>{resource}</b>")
            msg_lines.append(f"   💰 Amount: <b>{trx_amount:.6f} TRX</b>")
    
    elif contract_type == 'VoteWitnessContract':
        msg_lines.append("")
        # Voting for witnesses
        votes = event_data.get('votes', [])
        if votes:
            msg_lines.append(f"   🗳️ Voting for {len(votes)} witness(es)")
    
    elif contract_type in ['FreezeBalanceContract', 'UnfreezeBalanceContract', 
                            'FreezeBalanceV2Contract', 'UnfreezeBalanceV2Contract']:
        msg_lines.append("")
        frozen_amount = event.get('Amount', 0)
        if frozen_amount > 0:
            trx_amount = frozen_amount / 1_000_000
            resource = event_data.get('resource', 'ENERGY')
            msg_lines.append(f"   💎 Resource: <b>{resource}</b>")
            msg_lines.append(f"   💰 Amount: <b>{trx_amount:.6f} TRX</b>")
    
    elif contract_type == 'AccountCreateContract':
        msg_lines.append("")
        msg_lines.append(f"   🆕 New account created!")
    
    # Smart contract decoded info (check both EventData and direct smartContract)
    sc = event_data.get('smartContract') or event.get('smartContract', {})
    if sc:
        msg_lines.append("")
        msg_lines.append("🔍 <b>Smart Contract Details:</b>")
        
        method_name = sc.get('methodName')
        method_sig = sc.get('methodSignature')
        
        if method_name:
            msg_lines.append(f"   ⚙️ Method: <code>{method_name}</code>")
        elif method_sig:
            # Show hex signature if method name is unknown
            msg_lines.append(f"   ⚙️ Method: <code>0x{method_sig}</code> (unknown)")
        
        addresses = sc.get('addresses', [])
        if addresses:
            msg_lines.append(f"   📍 Param Addresses:")
            for addr in addresses[:3]:  # Limit to 3 addresses
                msg_lines.append(f"      • {format_address_link(addr)}")
        
        sc_amount = sc.get('amount')
        if sc_amount:
            # Convert token amount (assuming 6 decimals like USDT)
            try:
                token_amount = int(sc_amount) / 1_000_000
                msg_lines.append(f"   💵 Token Amount: <code>{token_amount:,.6f}</code>")
            except (ValueError, TypeError):
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
        
        await process_websocket_events(chat_id, address, ws_url, filter_sc=False)
                    
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


async def monitor_smart_contracts(chat_id: int, address: str):
    """Monitor smart contract events for an address via WebSocket"""
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
                
                logger.info(f"Created SC subscription {subscription_id} for address {address}")
        
        # Connect to WebSocket with subscription ID
        ws_url = f"{WS_BASE_URL}/api/v1/events/stream/{subscription_id}"
        logger.info(f"Connecting to WebSocket for SC monitoring: {ws_url}")
        
        await process_websocket_events(chat_id, address, ws_url, filter_sc=True)
                    
    except websockets.exceptions.WebSocketException as e:
        logger.error(f"WebSocket error for {address}: {e}")
        await bot.send_message(
            chat_id,
            f"❌ Connection lost for smart contract monitor:\n<code>{address}</code>\n\n"
            f"Error: {str(e)}",
            parse_mode="HTML"
        )
    except Exception as e:
        logger.error(f"Error monitoring smart contracts for {address}: {e}")
        await bot.send_message(
            chat_id,
            f"❌ Error monitoring smart contracts:\n<code>{address}</code>\n\n"
            f"Error: {str(e)}",
            parse_mode="HTML"
        )
    finally:
        # Clean up
        sc_key = f"{address}_SC"
        if chat_id in active_monitors and sc_key in active_monitors[chat_id]:
            del active_monitors[chat_id][sc_key]
            if not active_monitors[chat_id]:
                del active_monitors[chat_id]


async def monitor_all_smart_contracts(chat_id: int):
    """Monitor all smart contract events across the entire network"""
    subscription_id = None
    try:
        # Create subscription with empty address for global monitoring
        # Use contract type filter to only see smart contract interactions
        async with aiohttp.ClientSession() as session:
            async with session.post(
                f"{API_BASE_URL}/api/v1/subscriptions",
                json={
                    "address": "",  # Empty address = monitor all addresses
                    "filters": {
                        "contractTypes": ["TriggerSmartContract"]
                    }
                }
            ) as resp:
                if resp.status != 200 and resp.status != 201:
                    error_text = await resp.text()
                    logger.error(f"Failed to create global SC subscription: {error_text}")
                    await bot.send_message(
                        chat_id,
                        f"❌ Failed to create global SC monitoring subscription: {error_text}"
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
                
                logger.info(f"Created global SC subscription {subscription_id}")
        
        # Connect to WebSocket with subscription ID
        ws_url = f"{WS_BASE_URL}/api/v1/events/stream/{subscription_id}"
        logger.info(f"Connecting to WebSocket for global SC monitoring: {ws_url}")
        
        # Don't filter on bot side since API is already filtering by contract type
        await process_websocket_events(chat_id, "GLOBAL_SC", ws_url, filter_sc=False)
                    
    except websockets.exceptions.WebSocketException as e:
        logger.error(f"WebSocket error for global SC monitor: {e}")
        await bot.send_message(
            chat_id,
            f"❌ Connection lost for global smart contract monitor\n\n"
            f"Error: {str(e)}",
            parse_mode="HTML"
        )
    except Exception as e:
        logger.error(f"Error monitoring global smart contracts: {e}")
        await bot.send_message(
            chat_id,
            f"❌ Error monitoring global smart contracts\n\n"
            f"Error: {str(e)}",
            parse_mode="HTML"
        )
    finally:
        # Clean up
        if chat_id in active_monitors and "GLOBAL_SC" in active_monitors[chat_id]:
            del active_monitors[chat_id]["GLOBAL_SC"]
            if not active_monitors[chat_id]:
                del active_monitors[chat_id]


async def process_websocket_events(chat_id: int, address: str, ws_url: str, filter_sc: bool = False):
    """Process WebSocket events with optional smart contract filtering"""
    async with websockets.connect(ws_url) as websocket:
        monitor_type = "smart contract interactions" if filter_sc else "all transactions"
        await bot.send_message(
            chat_id,
            f"✅ Now monitoring {monitor_type} for:\n<code>{address}</code>\n\n"
            f"You'll receive notifications for all {'smart contract ' if filter_sc else ''}events.",
            parse_mode="HTML"
        )
        
        # Listen for events
        async for message in websocket:
            try:
                # Log raw message for debugging
                logger.debug(f"Received WebSocket message: {message[:200]}...")
                
                event = json.loads(message)
                logger.info(f"Parsed event: {event}")
                
                # If filtering for smart contracts only, check if this is a smart contract event
                if filter_sc:
                    contract_type = event.get('ContractType') or event.get('contractType')
                    event_data = event.get('EventData', {})
                    has_sc = event_data.get('smartContract') or event.get('smartContract')
                    
                    # Skip if not a smart contract transaction
                    if contract_type != 'TriggerSmartContract' and not has_sc:
                        logger.debug(f"Skipping non-SC event in SC filter mode")
                        continue
                
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


@dp.message(Command("start"))
async def cmd_start(message: Message):
    """Handle /start command"""
    welcome_text = """
👋 <b>Welcome to Mongotron Monitor Bot!</b>

This bot helps you monitor Tron blockchain addresses in real-time on <b>Nile Testnet</b>.

<b>Available commands:</b>

/monitor &lt;address&gt; - Monitor all transactions for an address
/monitor_sc &lt;address&gt; - Monitor only smart contract interactions
/monitor_allsc - Monitor all smart contracts globally
/stop_monitor &lt;address|global&gt; - Stop monitoring
/stop_all - Stop all active monitors
/list - List all active monitors
/help - Show this help message

<b>Examples:</b>
<code>/monitor TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf</code>
<code>/monitor_sc TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf</code> (Nile USDT - SC only)
<code>/monitor_allsc</code> (All SC activity - high volume!)

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


@dp.message(Command("monitor_sc"))
async def cmd_monitor_sc(message: Message):
    """Handle /monitor_sc command - monitor only smart contract interactions"""
    # Extract address from command
    parts = message.text.split(maxsplit=1)
    if len(parts) < 2:
        await message.answer(
            "❌ Please provide an address to monitor for smart contract interactions.\n\n"
            "<b>Usage:</b> <code>/monitor_sc &lt;address&gt;</code>\n\n"
            "<b>Examples:</b>\n"
            "<code>/monitor_sc TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf</code>\n"
            "<code>/monitor_sc TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf</code> (Nile USDT)\n\n"
            "<b>Note:</b> This will only show transactions involving smart contracts.\n"
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
    
    # Use a special key for SC monitors
    sc_key = f"{address}_SC"
    
    # Check if already monitoring
    if chat_id in active_monitors and sc_key in active_monitors[chat_id]:
        await message.answer(
            f"ℹ️ Already monitoring smart contract interactions for:\n<code>{address}</code>",
            parse_mode="HTML"
        )
        return
    
    # Start monitoring
    await message.answer(
        f"🔄 Starting smart contract monitor for:\n<code>{address}</code>\n\n"
        f"Please wait...",
        parse_mode="HTML"
    )
    
    # Create monitoring task
    task = asyncio.create_task(monitor_smart_contracts(chat_id, address))
    
    # Store the task with SC key
    if chat_id not in active_monitors:
        active_monitors[chat_id] = {}
    active_monitors[chat_id][sc_key] = task
    
    logger.info(f"Started SC monitoring {address} for chat {chat_id}")


@dp.message(Command("monitor_allsc"))
async def cmd_monitor_allsc(message: Message):
    """Handle /monitor_allsc command - monitor all smart contract interactions globally"""
    chat_id = message.chat.id
    
    # Check if already monitoring
    if chat_id in active_monitors and "GLOBAL_SC" in active_monitors[chat_id]:
        await message.answer(
            "ℹ️ Already monitoring global smart contract activity.",
            parse_mode="HTML"
        )
        return
    
    # Start monitoring
    await message.answer(
        "🔄 Starting global smart contract monitor...\n\n"
        "📡 This will show smart contract interactions across the network via USDT contract.\n\n"
        "⚠️ Note: High volume of events expected!",
        parse_mode="HTML"
    )
    
    # Create monitoring task
    task = asyncio.create_task(monitor_all_smart_contracts(chat_id))
    
    # Store the task with GLOBAL_SC key
    if chat_id not in active_monitors:
        active_monitors[chat_id] = {}
    active_monitors[chat_id]["GLOBAL_SC"] = task
    
    logger.info(f"Started global SC monitoring for chat {chat_id}")


@dp.message(Command("stop_monitor"))
async def cmd_stop_monitor(message: Message):
    """Handle /stop_monitor command"""
    # Extract address from command
    parts = message.text.split(maxsplit=1)
    if len(parts) < 2:
        await message.answer(
            "❌ Please provide an address to stop monitoring.\n\n"
            "<b>Usage:</b> <code>/stop_monitor &lt;address|global&gt;</code>\n\n"
            "Use <code>/stop_monitor global</code> to stop global SC monitor.",
            parse_mode="HTML"
        )
        return
    
    address = parts[1].strip()
    chat_id = message.chat.id
    
    # Check for global SC monitor
    if address.lower() == "global":
        if chat_id in active_monitors and "GLOBAL_SC" in active_monitors[chat_id]:
            task = active_monitors[chat_id]["GLOBAL_SC"]
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass
            
            if "GLOBAL_SC" in active_monitors[chat_id]:
                del active_monitors[chat_id]["GLOBAL_SC"]
            if not active_monitors[chat_id]:
                del active_monitors[chat_id]
            
            await message.answer("✅ Stopped global smart contract monitor", parse_mode="HTML")
            logger.info(f"Stopped global SC monitor for chat {chat_id}")
        else:
            await message.answer("ℹ️ Global SC monitor is not running.", parse_mode="HTML")
        return
    
    # Check for both regular and SC monitors
    sc_key = f"{address}_SC"
    address_key = None
    monitor_type = ""
    
    if chat_id in active_monitors:
        if address in active_monitors[chat_id]:
            address_key = address
            monitor_type = ""
        elif sc_key in active_monitors[chat_id]:
            address_key = sc_key
            monitor_type = " (SC only)"
    
    if not address_key:
        await message.answer(
            f"ℹ️ Not currently monitoring address:\n<code>{address}</code>",
            parse_mode="HTML"
        )
        return
    
    # Stop the monitoring task
    task = active_monitors[chat_id][address_key]
    task.cancel()
    
    try:
        await task
    except asyncio.CancelledError:
        pass
    
    # Remove from active monitors
    del active_monitors[chat_id][address_key]
    if not active_monitors[chat_id]:
        del active_monitors[chat_id]
    
    await message.answer(
        f"✅ Stopped monitoring{monitor_type} address:\n<code>{address}</code>",
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
        if address in active_monitors[chat_id]:  # Check if still exists
            task = active_monitors[chat_id][address]
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass
    
    # Clear all monitors for this chat (check if still exists)
    if chat_id in active_monitors:
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
    for i, address_key in enumerate(addresses, 1):
        if address_key == "GLOBAL_SC":
            # Global SC monitor
            msg += f"{i}. 🌐 Global Smart Contract Monitor\n"
        elif address_key.endswith('_SC'):
            # Smart contract only monitor
            address = address_key[:-3]  # Remove _SC suffix
            msg += f"{i}. <code>{address}</code> 🔍 (SC only)\n"
        else:
            # Regular monitor
            msg += f"{i}. <code>{address_key}</code>\n"
    
    msg += f"\n💡 Use <code>/stop_monitor &lt;address&gt;</code> to stop a specific monitor"
    msg += f"\n💡 Use <code>/stop_monitor global</code> to stop global SC monitor"
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
