#!/usr/bin/env python3
"""Test address conversion with real event data"""

import sys
sys.path.insert(0, '/home/user0/Github/mongotron')
from event_monitor import hex_to_tron_address

# Real addresses from the captured event
from_hex = "41d3682962027e721c5247a9faf7865fe4a71d5438"
to_hex = "41eca9bc828a3005b9a3b909f2cc5c2a54794de05f"

print("="*80)
print("TRANSACTION EVENT - TriggerSmartContract")
print("="*80)
print(f"TX Hash: bc5f7798fa35b8899a990e3cabd2baaccd3e69be10b934359864617fce1c4821")
print(f"Block:   61090262")
print(f"Time:    2025-10-05 19:23:24")
print(f"Success: True")

print(f"\n{'─'*80}")
print(f"📊 ADDRESSES:")
print(f"{'─'*80}")
print(f"   From (hex):  {from_hex}")
print(f"   From:        {hex_to_tron_address(from_hex)}")
print(f"   To (hex):    {to_hex}")
print(f"   To:          {hex_to_tron_address(to_hex)}")
print(f"{'='*80}\n")

print("\n✅ This is how the monitor will display events with human-readable addresses!")
