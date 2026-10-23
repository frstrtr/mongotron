# MongoTron documentation

Everything except the root [README](../README.md) and [CHANGELOG](../CHANGELOG.md) lives here,
in four folders. Commands in these pages run from the repository root.

| Folder | What it holds |
|---|---|
| [architecture/](architecture/) | How the service is built |
| [guides/](guides/) | How to install, start, test, deploy and tune it |
| [reference/](reference/) | API and feature reference |
| [notes/](notes/) | Development history: bug fix write-ups, session summaries, test and coverage reports. Kept as written at the time, so hosts, versions and status lines can be out of date. |

## Architecture

- [IMPLEMENTATION_SUMMARY.md](architecture/IMPLEMENTATION_SUMMARY.md): API server components, data flow, configuration
- [ABI_DECODING_IMPLEMENTATION.md](architecture/ABI_DECODING_IMPLEMENTATION.md): smart contract parameter decoding and address detection

## Guides

- [QUICK_START.md](guides/QUICK_START.md): first commands against a running server
- [STARTUP_GUIDE.md](guides/STARTUP_GUIDE.md): start MongoDB, the Tron node and the API server
- [HOW_TO_START.md](guides/HOW_TO_START.md): short start checklist
- [INSTALL_SCRIPT.md](guides/INSTALL_SCRIPT.md): `scripts/install-prerequisites.sh`
- [INFRASTRUCTURE.md](guides/INFRASTRUCTURE.md): hardware, ZFS, MongoDB and security setup
- [DEPLOYMENT.md](guides/DEPLOYMENT.md): Docker and Kubernetes deployment
- [PERFORMANCE_TUNING.md](guides/PERFORMANCE_TUNING.md): configuration and scaling
- [TEST_GUIDE.md](guides/TEST_GUIDE.md): unit and integration tests, `scripts/run_tests.sh`
- [EVENT_MONITOR_README.md](guides/EVENT_MONITOR_README.md): the Python event monitor in `examples/python/`
- [EVENT_LOGGING_GUIDE.md](guides/EVENT_LOGGING_GUIDE.md): the event monitor's log files
- [CLIENT_README.md](guides/CLIENT_README.md): the Python API test client in `examples/python/`

## Reference

- [API_SERVER_README.md](reference/API_SERVER_README.md): REST and WebSocket endpoints, filters, event structure
- [API.md](reference/API.md): API overview, rate limits and error codes
- [QUICK_REFERENCE.md](reference/QUICK_REFERENCE.md): one-page command card
- [SMART_CONTRACT_DECODING_MONITOR.md](reference/SMART_CONTRACT_DECODING_MONITOR.md): decoded TRC20 functions in the event monitor
- [SMART_CONTRACT_QUICK_REF.md](reference/SMART_CONTRACT_QUICK_REF.md): short version of the above

The Porto API integration (watch list, signed webhooks, skipped blocks) is documented in the
root [README](../README.md#porto-api-integration).

## Notes (history)

- Releases and status: [RELEASE_NOTES.md](notes/RELEASE_NOTES.md), [INIT_COMPLETE.md](notes/INIT_COMPLETE.md), [PREREQUISITES_INSTALLED.md](notes/PREREQUISITES_INSTALLED.md), [SERVER_STATUS.md](notes/SERVER_STATUS.md), [DOCUMENTATION_SUMMARY.md](notes/DOCUMENTATION_SUMMARY.md)
- Bug fixes: [BUGFIX_BLOCK_PROCESSING.md](notes/BUGFIX_BLOCK_PROCESSING.md), [BUGFIX_DOUBLE_DELETION.md](notes/BUGFIX_DOUBLE_DELETION.md), [SUBSCRIPTION_DELETION_FIX.md](notes/SUBSCRIPTION_DELETION_FIX.md), [WEBSOCKET_FIX.md](notes/WEBSOCKET_FIX.md)
- Features: [ADDRESS_CONVERSION.md](notes/ADDRESS_CONVERSION.md), [ADDRESS_CONVERSION_SUCCESS.md](notes/ADDRESS_CONVERSION_SUCCESS.md), [ENHANCED_MONITOR_SUMMARY.md](notes/ENHANCED_MONITOR_SUMMARY.md), [MVP_SMART_CONTRACT_DECODING.md](notes/MVP_SMART_CONTRACT_DECODING.md), [FILES_UPDATED_SUMMARY.md](notes/FILES_UPDATED_SUMMARY.md), [SESSION_SUMMARY.md](notes/SESSION_SUMMARY.md)
- Tests: [TEST_IMPLEMENTATION_SUMMARY.md](notes/TEST_IMPLEMENTATION_SUMMARY.md), [TEST_RESULTS.md](notes/TEST_RESULTS.md), [COVERAGE_IMPROVEMENT_REPORT.md](notes/COVERAGE_IMPROVEMENT_REPORT.md)

## Repository layout

```
cmd/            entry points: api-server, mvp, cli, migrate
internal/       service packages (api, blockchain, config, storage, subscription, webhook)
pkg/            shared packages (logger)
configs/        runtime configs: mongotron.yaml (read by deployed hosts), mongotron-oplx2.yaml,
                mongotron.yml (Docker image), .env.example
deployments/    docker/ (Dockerfile, compose files) and kubernetes/ manifests
scripts/        build, test, deploy, benchmark, install-prerequisites, run_tests, start, test_api
tests/          Go tests: unit/, integration/ (integration/api needs a live server and
                MONGOTRON_INTEGRATION=1), performance/
examples/       python/ (event monitor, API test client), js/ (WebSocket client),
                telegram-bot/ (demo bot)
tools/          Go module that pins development tools
docs/           this documentation
archive/        old copies kept for history, not used by anything (see archive/README.md)
```
