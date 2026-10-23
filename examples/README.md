# examples/

Clients and demos that talk to a running MongoTron API server (default `http://localhost:8080`).
They are not part of the Go build. Commands below run from the repository root.

| Path | What it is | Docs |
|---|---|---|
| `python/event_monitor.py` | Subscribes to an address and prints decoded events from the WebSocket stream | [EVENT_MONITOR_README.md](../docs/guides/EVENT_MONITOR_README.md) |
| `python/test_client.py` | Exercises the REST and WebSocket API and prints a report | [CLIENT_README.md](../docs/guides/CLIENT_README.md) |
| `python/test_address_display.py`, `python/test_smart_contract_display.py` | Small checks of the monitor's address and contract display helpers | |
| `python/demo_monitor.sh` | Prints usage examples for the event monitor | |
| `python/requirements.txt` | Python dependencies of the scripts above | |
| `js/test_websocket.js` | Node.js WebSocket client for one subscription (needs the `ws` package) | |
| `telegram-bot/` | Demo Telegram bot: watches addresses through the API and sends transaction notifications to a chat | [README.md](telegram-bot/README.md) |

```bash
pip install -r examples/python/requirements.txt
python examples/python/event_monitor.py --help
node examples/js/test_websocket.js <subscription_id>
```
