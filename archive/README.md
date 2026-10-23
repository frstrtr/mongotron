# archive/

Old copies kept so their history stays in the repository. Nothing here is used: no build,
test, CI job, script or deployment reads these files, and they are not maintained.

| File | What it is |
|---|---|
| `event_monitor.py.broken` | Corrupted copy of the Python event monitor (a syntax error at line 15). The working monitor is `examples/python/event_monitor.py`. |
| `event_monitor_fixed.py` | Older partial copy of the event monitor; it does not compile (IndentationError at line 65). |
| `event_monitor_fix.py` | Standalone test of the hex to base58 address conversion, written while that feature was added (see `docs/notes/ADDRESS_CONVERSION.md`). Runs on its own. |
| `server.pid` | PID file of a local API server run, committed by mistake. `*.pid` is now in `.gitignore`. |
| `configs/mongotron.yaml.bak` | Backup of an early config for a Nile test VM. Its MongoDB password and host name are replaced by placeholders. Live configs stay in `configs/`. |
