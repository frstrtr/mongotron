# MongoTron Performance Tuning Guide

## Configuration Optimization

### Worker Pool Sizing
```yaml
worker_pool:
  workers: 1000  # Adjust based on CPU cores (500-2000)
  queue_size: 100000  # Buffer size for pending jobs
```

**Recommendations:**
- CPU cores * 100-200 for optimal performance
- Monitor CPU usage and adjust accordingly

### MongoDB Connection Pool
```yaml
database:
  mongodb:
    options:
      max_pool_size: 100  # Maximum connections
      min_pool_size: 10   # Minimum idle connections
```

**Recommendations:**
- Set max_pool_size to workers / 10
- Monitor connection usage with MongoDB stats

### Memory Management
```bash
MONGOTRON_MEMORY_LIMIT=4GB
MONGOTRON_GC_PERCENT=100  # Default Go GC
```

**Tuning Tips:**
- Lower GC_PERCENT (50-75) for more frequent GC, lower memory usage
- Higher GC_PERCENT (150-200) for better throughput, higher memory usage

## Performance Metrics

### Key Metrics to Monitor

1. **Event Processing Time**: Should be < 1ms
2. **Addresses Monitored**: Current active subscriptions
3. **CPU Usage**: Should stay < 70% for autoscaling headroom
4. **Memory Usage**: ~1KB per address + overhead
5. **MongoDB Query Time**: Should be < 10ms

## Benchmarking

Run benchmarks:
```bash
make benchmark
```

Compare results against baseline metrics in README.md.

## Horizontal Scaling

### Kubernetes HPA
The HPA will automatically scale based on:
- CPU utilization > 70%
- Memory utilization > 80%
- Addresses monitored > 10,000 per pod

### Load Balancing
Use session affinity for WebSocket connections:
```yaml
sessionAffinity: ClientIP
```

## Database Optimization

### Index Creation
Ensure these indexes exist:
```javascript
db.addresses.createIndex({ "address": 1 })
db.transactions.createIndex({ "block_number": -1 })
db.events.createIndex({ "subscription_id": 1, "timestamp": -1 })
```

### Query Optimization
- Use projection to limit returned fields
- Implement pagination for large result sets
- Use aggregation pipeline for complex queries

## Network Optimization

### gRPC Connection Pool
```yaml
blockchain:
  tron:
    connection:
      keep_alive: 30s
      timeout: 30s
```

Maintain persistent connections to Tron node.

## Profiling

### CPU Profile
```bash
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

### Memory Profile
```bash
curl http://localhost:6060/debug/pprof/heap > mem.prof
go tool pprof mem.prof
```

### Goroutine Analysis
```bash
curl http://localhost:6060/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
```
