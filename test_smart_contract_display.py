#!/usr/bin/env python3
"""
Test the enhanced smart contract decoding display
Shows the last few events with decoded information
"""

import requests
import json
from event_monitor import hex_to_tron_address

def test_smart_contract_display():
    """Fetch and display recent events with smart contract decoding"""
    
    print("=" * 80)
    print("SMART CONTRACT DECODING TEST")
    print("=" * 80)
    print()
    
    # Fetch recent events
    url = "http://localhost:8080/api/v1/events?limit=3"
    response = requests.get(url)
    
    if response.status_code != 200:
        print(f"❌ Error fetching events: {response.status_code}")
        return
    
    data = response.json()
    events = data.get("events", [])
    
    if not events:
        print("No events found.")
        return
    
    print(f"Found {len(events)} recent events\n")
    
    for i, event in enumerate(events, 1):
        event_data = event.get("data", {})
        smart_contract = event_data.get("eventData", {}).get("smartContract")
        
        print(f"\n{'='*80}")
        print(f"EVENT #{i}")
        print(f"{'='*80}")
        print(f"TX Hash:      {event.get('txHash', 'N/A')[:40]}...")
        print(f"Block:        {event.get('blockNumber', 'N/A')}")
        print(f"Type:         {event.get('type', 'N/A')}")
        print(f"Success:      {event_data.get('success', False)}")
        
        print(f"\n{'-'*80}")
        print(f"BASIC TRANSACTION:")
        print(f"{'-'*80}")
        from_hex = event_data.get('from', 'N/A')
        to_hex = event_data.get('to', 'N/A')
        print(f"From (hex):   {from_hex}")
        print(f"From:         {hex_to_tron_address(from_hex) if from_hex != 'N/A' else 'N/A'}")
        print(f"To (hex):     {to_hex}")
        print(f"To:           {hex_to_tron_address(to_hex) if to_hex != 'N/A' else 'N/A'}")
        
        if smart_contract:
            print(f"\n{'-'*80}")
            print(f"🔍 SMART CONTRACT DECODED:")
            print(f"{'-'*80}")
            
            method_name = smart_contract.get("methodName", "Unknown")
            method_sig = smart_contract.get("methodSignature", "N/A")
            addresses = smart_contract.get("addresses", [])
            amount = smart_contract.get("amount")
            parameters = smart_contract.get("parameters", {})
            
            print(f"Function:     {method_name}")
            print(f"Signature:    0x{method_sig}")
            
            if "transfer(address,uint256)" in method_name:
                print(f"\n💸 TOKEN TRANSFER:")
                if len(addresses) > 0:
                    recipient = addresses[0]
                    recipient_readable = hex_to_tron_address(recipient)
                    print(f"   To (hex):     {recipient}")
                    print(f"   To:           {recipient_readable}")
                if amount:
                    try:
                        amount_int = int(amount)
                        amount_formatted = amount_int / 1_000_000
                        print(f"   Amount:       {amount_formatted:,.6f} tokens")
                        print(f"   Amount (raw): {amount}")
                    except:
                        print(f"   Amount:       {amount}")
            
            elif "transferFrom(address,address,uint256)" in method_name:
                print(f"\n💸 TOKEN TRANSFER (Approved):")
                if len(addresses) >= 2:
                    from_addr = addresses[0]
                    to_addr = addresses[1]
                    print(f"   From (hex):   {from_addr}")
                    print(f"   From:         {hex_to_tron_address(from_addr)}")
                    print(f"   To (hex):     {to_addr}")
                    print(f"   To:           {hex_to_tron_address(to_addr)}")
                if amount:
                    try:
                        amount_int = int(amount)
                        amount_formatted = amount_int / 1_000_000
                        print(f"   Amount:       {amount_formatted:,.6f} tokens")
                    except:
                        print(f"   Amount:       {amount}")
            
            else:
                print(f"\nAddresses in parameters:")
                for addr in addresses:
                    print(f"   {addr}")
                    print(f"   {hex_to_tron_address(addr)}")
                if parameters:
                    print(f"\nOther parameters:")
                    for key, value in parameters.items():
                        print(f"   {key}: {value}")
        else:
            print(f"\n{'-'*80}")
            print(f"⚠️  No smart contract decoding available")
            print(f"{'-'*80}")
    
    print(f"\n{'='*80}")
    print("TEST COMPLETE")
    print(f"{'='*80}\n")

if __name__ == "__main__":
    test_smart_contract_display()
