#!/usr/bin/env python3
"""
MongoTron Event Monitor
Subscribes to blockchain events and displays real-time updates via WebSocket
"""

import requests
import json
import time
import sys
import signal
import logging
import hashlib
import base58
from datetime import datetime
from websocket import WebSocketApp
import threading

# USDT contract on Tron Nile Testnet
# Note: This is a testnet USDT contract. For mainnet, use: TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t
USDT_CONTRACT = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"  # Tron Nile Testnet USDT

def hex_to_tron_address(hex_address: str) -> str:
    """
    Convert hex address to Tron base58 address
    
    Args:
        hex_address: Hex address (with or without 0x prefix, with or without 41 prefix)
    
    Returns:
        Base58 encoded Tron address (T...)
    """
    try:
        # Remove 0x prefix if present
        if hex_address.startswith('0x') or hex_address.startswith('0X'):
            hex_address = hex_address[2:]
        
        # Remove padding zeros if present (from topics)
        hex_address = hex_address.lstrip('0')
        
        # Add 41 prefix if not present (Tron mainnet/testnet prefix)
        if not hex_address.startswith('41'):
            hex_address = '41' + hex_address
        
        # Ensure even length
        if len(hex_address) % 2 != 0:
            hex_address = '0' + hex_address
        
        # Convert hex to bytes
        addr_bytes = bytes.fromhex(hex_address)
        
        # Calculate checksum (double SHA256)
        hash1 = hashlib.sha256(addr_bytes).digest()
        hash2 = hashlib.sha256(hash1).digest()
        checksum = hash2[:4]
        
        # Append checksum and encode to base58
        addr_with_checksum = addr_bytes + checksum
        base58_addr = base58.b58encode(addr_with_checksum).decode('utf-8')
        
        return base58_addr
    except Exception as e:
        # If conversion fails, return original hex
        return f"{hex_address} (conversion failed: {e})"

# Test the converter
if __name__ == "__main__":
    # Test addresses from your example
    test_from = "41d3682962027e721c5247a9faf7865fe4a71d5438"
    test_to = "41eca9bc828a3005b9a3b909f2cc5c2a54794de05f"
    
    print(f"From hex: {test_from}")
    print(f"From readable: {hex_to_tron_address(test_from)}")
    print(f"\nTo hex: {test_to}")
    print(f"To readable: {hex_to_tron_address(test_to)}")
