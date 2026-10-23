# MongoTron Infrastructure Guide

Complete guide for setting up production-grade infrastructure for MongoTron with optimized storage, database configuration, and monitoring.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Hardware Requirements](#hardware-requirements)
- [Storage Setup (ZFS on NVMe)](#storage-setup-zfs-on-nvme)
- [MongoDB Installation & Configuration](#mongodb-installation--configuration)
- [Network Configuration](#network-configuration)
- [Security Hardening](#security-hardening)
- [Performance Tuning](#performance-tuning)
- [Monitoring & Alerting](#monitoring--alerting)
- [Backup & Recovery](#backup--recovery)
- [Scaling Strategies](#scaling-strategies)
- [Troubleshooting](#troubleshooting)

---

## Overview

MongoTron's infrastructure is designed for high-performance blockchain monitoring with the following components:

- **Storage Layer**: ZFS on NVMe for optimal I/O performance
- **Database**: MongoDB 7.0+ with WiredTiger engine
- **Application**: Go-based microservice with goroutine worker pools
- **Monitoring**: Prometheus + Grafana for metrics and alerting

### Production Infrastructure Example

```
┌─────────────────────────────────────────────────────────────┐
│                    Production Environment                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐   │
│  │  MongoTron   │─────►│   MongoDB    │─────►│   ZFS    │   │
│  │  Application │      │   7.0.25     │      │  on NVMe │   │
│  │  (Go 1.24)   │      │              │      │  100GB+  │   │
│  └──────────────┘      └──────────────┘      └──────────┘   │
│         │                     │                     │       │
│         │                     │                     │       │
│         ▼                     ▼                     ▼       │
│  ┌──────────────────────────────────────────────────────┐   │
│  │           Monitoring (Prometheus + Grafana)          │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Architecture

### Component Layout

| Component | Purpose | Technology | Location |
|-----------|---------|------------|----------|
| Application | Blockchain monitoring | Go 1.24+ | Any server |
| Database | Data persistence | MongoDB 7.0+ | Dedicated VM/server |
| Storage | Database storage | ZFS on NVMe | Same as MongoDB |
| Cache | Optional caching | Redis 7.0+ | Optional |
| Monitoring | Metrics & alerts | Prometheus/Grafana | Any server |
| Load Balancer | Traffic distribution | NGINX/HAProxy | Edge |

### Data Flow

```
Tron Network (gRPC)
      ↓
MongoTron Worker Pool (50K+ goroutines)
      ↓
Event Processing Engine
      ↓
MongoDB (ZFS Storage)
      ↓
API Endpoints (REST/WebSocket/gRPC)
      ↓
Client Applications
```

---

## Hardware Requirements

### Minimum Requirements (Development)

| Component | Specification |
|-----------|--------------|
| CPU | 4 cores @ 2.5GHz |
| RAM | 8GB |
| Storage | 50GB SSD |
| Network | 100 Mbps |

### Recommended (Production)

| Component | Specification |
|-----------|--------------|
| CPU | 8-16 cores @ 3.0GHz+ |
| RAM | 32GB+ |
| Storage | 500GB+ NVMe SSD |
| Network | 1 Gbps+ |

### Optimal (High-Traffic Production)

| Component | Specification |
|-----------|--------------|
| CPU | 32+ cores @ 3.5GHz+ |
| RAM | 64GB+ |
| Storage | 1TB+ NVMe SSD (multiple drives for RAID) |
| Network | 10 Gbps+ |

---

## Storage Setup (ZFS on NVMe)

### Why ZFS for MongoDB?

ZFS provides several benefits for database workloads:

✅ **Built-in Compression** - LZ4 compression (2-3x savings)  
✅ **Copy-on-Write** - Consistent snapshots without downtime  
✅ **Data Integrity** - Checksumming prevents silent corruption  
✅ **Snapshots** - Instant point-in-time backups  
✅ **Performance** - Optimized for database I/O patterns  

### Installation

#### Ubuntu/Debian

```bash
# Install ZFS utilities
sudo apt update
sudo apt install -y zfsutils-linux

# Load ZFS module
sudo modprobe zfs
```

#### RHEL/CentOS/Rocky

```bash
# Install ZFS repository
sudo yum install -y epel-release
sudo yum install -y https://zfsonlinux.org/epel/zfs-release-2-2.el$(rpm -E %{rhel}).noarch.rpm

# Install ZFS
sudo yum install -y zfs

# Load ZFS module
sudo modprobe zfs
```

### Detecting NVMe Drives

```bash
# List all NVMe devices
lsblk -d -o NAME,SIZE,TYPE | grep nvme

# Example output:
# nvme0n1  100G disk
# nvme1n1  500G disk

# Get detailed NVMe information
sudo nvme list

# Check device is not in use
lsblk -o NAME,MOUNTPOINT,SIZE /dev/nvme0n1
```

### Creating ZFS Pool

#### Basic Pool Creation

```bash
# Create pool named 'mongopool' on /dev/nvme0n1
sudo zpool create -f mongopool /dev/nvme0n1

# Verify pool creation
sudo zpool status mongopool
sudo zpool list mongopool
```

#### Production Pool (Recommended)

```bash
# Create pool with MongoDB-optimized settings
sudo zpool create -f \
  -o ashift=12 \
  -O compression=lz4 \
  -O atime=off \
  -O relatime=on \
  -O recordsize=16K \
  -O logbias=latency \
  -O xattr=sa \
  -O dnodesize=auto \
  -O mountpoint=none \
  mongopool /dev/nvme0n1

# Explanation of settings:
# -o ashift=12         : 4K sector alignment (optimal for modern SSDs)
# -O compression=lz4   : Fast compression (low CPU, high throughput)
# -O atime=off         : Don't update access times (performance)
# -O relatime=on       : Relative access time (better than atime)
# -O recordsize=16K    : Optimal for MongoDB (default 128K is too large)
# -O logbias=latency   : Optimize for low latency over throughput
# -O xattr=sa          : Store extended attributes in system attributes
# -O dnodesize=auto    : Automatic dnode sizing
# -O mountpoint=none   : Don't mount pool directly (use datasets)
```

#### RAID Configurations

**RAID-0 (Striping - Maximum Performance)**
```bash
# Stripe across multiple NVMe drives
sudo zpool create -f mongopool \
  /dev/nvme0n1 /dev/nvme1n1 /dev/nvme2n1
# Pros: 3x speed, 3x capacity
# Cons: No redundancy, any drive failure = total data loss
```

**RAID-1 (Mirroring - Maximum Reliability)**
```bash
# Mirror drives for redundancy
sudo zpool create -f mongopool mirror \
  /dev/nvme0n1 /dev/nvme1n1
# Pros: Full redundancy, survives 1 drive failure
# Cons: 50% capacity, 50% write speed
```

**RAID-10 (Striped Mirrors - Best Balance)**
```bash
# Stripe across mirrored pairs (requires 4+ drives)
sudo zpool create -f mongopool \
  mirror /dev/nvme0n1 /dev/nvme1n1 \
  mirror /dev/nvme2n1 /dev/nvme3n1
# Pros: Good speed, good redundancy
# Cons: 50% capacity
```

### Creating MongoDB Dataset

```bash
# Create dataset for MongoDB with optimized settings
sudo zfs create \
  -o mountpoint=/var/lib/mongodb \
  -o recordsize=16K \
  -o primarycache=metadata \
  -o compression=lz4 \
  -o atime=off \
  -o exec=off \
  -o setuid=off \
  mongopool/mongodb

# Explanation:
# -o recordsize=16K        : Matches MongoDB's data structure
# -o primarycache=metadata : Cache only metadata (saves RAM)
# -o compression=lz4       : Fast compression
# -o atime=off            : No access time updates
# -o exec=off             : No executable files allowed
# -o setuid=off           : No setuid binaries allowed

# Set proper permissions
sudo mkdir -p /var/lib/mongodb
sudo chown -R mongodb:mongodb /var/lib/mongodb
sudo chmod 750 /var/lib/mongodb

# Verify dataset
sudo zfs list mongopool/mongodb
sudo df -h /var/lib/mongodb
```

### ZFS Pool Management

#### Viewing Pool Status

```bash
# Detailed status
sudo zpool status mongopool

# Capacity and usage
sudo zpool list mongopool

# I/O statistics
sudo zpool iostat mongopool 1

# Dataset properties
sudo zfs get all mongopool/mongodb
```

#### Performance Monitoring

```bash
# Real-time I/O statistics
sudo zpool iostat -v mongopool 1

# ARC (Adaptive Replacement Cache) statistics
cat /proc/spl/kstat/zfs/arcstats | grep -E "^(size|c_max|hits|misses)"

# Detailed performance metrics
sudo zpool iostat -lpv mongopool 5
```

#### Tuning ZFS for MongoDB

**Increase ARC Size** (if you have plenty of RAM)
```bash
# Set ARC max to 8GB (adjust based on available RAM)
echo 8589934592 | sudo tee /sys/module/zfs/parameters/zfs_arc_max

# Make permanent (add to /etc/modprobe.d/zfs.conf)
echo "options zfs zfs_arc_max=8589934592" | sudo tee /etc/modprobe.d/zfs.conf
```

**Disable ZFS Prefetch** (for random I/O workloads like MongoDB)
```bash
echo 0 | sudo tee /sys/module/zfs/parameters/zfs_prefetch_disable
```

**Tune TXG (Transaction Group) Timeout**
```bash
# Set to 5 seconds (default is 5, range 1-60)
echo 5 | sudo tee /sys/module/zfs/parameters/zfs_txg_timeout
```

### ZFS Maintenance

#### Scrubbing (Data Integrity Checks)

```bash
# Start a scrub (check all data for corruption)
sudo zpool scrub mongopool

# Check scrub status
sudo zpool status mongopool

# Schedule weekly scrubs (add to /etc/cron.weekly/zfs-scrub)
sudo tee /etc/cron.weekly/zfs-scrub << 'EOF'
#!/bin/bash
zpool scrub mongopool
EOF
sudo chmod +x /etc/cron.weekly/zfs-scrub
```

#### Snapshots

```bash
# Create snapshot
sudo zfs snapshot mongopool/mongodb@$(date +%Y%m%d-%H%M%S)

# List snapshots
sudo zfs list -t snapshot

# Delete snapshot
sudo zfs destroy mongopool/mongodb@20251005-120000

# Rollback to snapshot (WARNING: loses all data after snapshot)
sudo zfs rollback mongopool/mongodb@20251005-120000

# Clone snapshot (creates writeable copy)
sudo zfs clone mongopool/mongodb@20251005-120000 mongopool/mongodb-clone
```

#### Automated Snapshots

```bash
# Install zfs-auto-snapshot (Ubuntu/Debian)
sudo apt install -y zfs-auto-snapshot

# Enable auto-snapshots for dataset
sudo zfs set com.sun:auto-snapshot=true mongopool/mongodb

# Configure retention (keep 24 hourly, 7 daily, 4 weekly, 12 monthly)
# Edit /etc/default/zfs-auto-snapshot
```

---

## MongoDB Installation & Configuration

### Installation

#### Ubuntu 24.04 (Noble) / 22.04 (Jammy)

```bash
# Import GPG key
curl -fsSL https://www.mongodb.org/static/pgp/server-7.0.asc | \
  sudo gpg -o /usr/share/keyrings/mongodb-server-7.0.gpg --dearmor

# Add MongoDB repository (use jammy for both 22.04 and 24.04)
echo "deb [ arch=amd64,arm64 signed-by=/usr/share/keyrings/mongodb-server-7.0.gpg ] \
  https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/7.0 multiverse" | \
  sudo tee /etc/apt/sources.list.d/mongodb-org-7.0.list

# Update and install
sudo apt update
sudo apt install -y mongodb-org

# Start and enable service
sudo systemctl start mongod
sudo systemctl enable mongod

# Verify installation
mongod --version
```

#### RHEL/CentOS/Rocky 8/9

```bash
# Create repository file
sudo tee /etc/yum.repos.d/mongodb-org-7.0.repo << 'EOF'
[mongodb-org-7.0]
name=MongoDB Repository
baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/7.0/x86_64/
gpgcheck=1
enabled=1
gpgkey=https://www.mongodb.org/static/pgp/server-7.0.asc
EOF

# Install
sudo yum install -y mongodb-org

# Start and enable
sudo systemctl start mongod
sudo systemctl enable mongod
```

### MongoDB Configuration for ZFS

Create optimized `/etc/mongod.conf`:

```yaml
# MongoDB Configuration for MongoTron on ZFS
# /etc/mongod.conf

# Storage Engine Configuration
storage:
  dbPath: /var/lib/mongodb
  directoryPerDB: true
  engine: wiredTiger
  wiredTiger:
    engineConfig:
      # Cache size (50-60% of available RAM recommended)
      cacheSizeGB: 2
      
      # Disable WiredTiger compression (ZFS handles it)
      journalCompressor: none
      directoryForIndexes: true
      
    collectionConfig:
      # Disable collection compression (ZFS handles it)
      blockCompressor: none
      
    indexConfig:
      # Disable index prefix compression for better performance
      prefixCompression: false

# System Logging
systemLog:
  destination: file
  logAppend: true
  path: /var/log/mongodb/mongod.log
  # Set log level (0=quiet, 1=info, 2=debug, 3+=verbose)
  verbosity: 0
  component:
    storage:
      journal:
        verbosity: 0

# Network Interfaces
net:
  port: 27017
  # Bind to all interfaces (use specific IP in production)
  bindIp: 0.0.0.0
  # Enable IPv6
  ipv6: false
  # Maximum incoming connections
  maxIncomingConnections: 10000

# Process Management
processManagement:
  timeZoneInfo: /usr/share/zoneinfo
  fork: false

# Security
security:
  # Enable authorization
  authorization: enabled
  # TLS/SSL configuration (recommended for production)
  # tls:
  #   mode: requireTLS
  #   certificateKeyFile: /etc/ssl/mongodb.pem
  #   CAFile: /etc/ssl/ca.pem

# Operation Profiling
operationProfiling:
  # Profile slow operations
  mode: slowOp
  # Threshold in milliseconds
  slowOpThresholdMs: 100
  slowOpSampleRate: 1.0

# Replication (for production clusters)
# replication:
#   replSetName: mongotron-rs
#   oplogSizeMB: 10240

# Sharding (for very large deployments)
# sharding:
#   clusterRole: shardsvr

# SetParameter (advanced tuning)
setParameter:
  # Increase connection pool
  connPoolMaxConnsPerHost: 200
  # Enable fast shutdown
  enableFastShutdown: true
```

Apply configuration:

```bash
# Backup original config
sudo cp /etc/mongod.conf /etc/mongod.conf.backup

# Create new config (paste above content)
sudo nano /etc/mongod.conf

# Set permissions
sudo chown mongodb:mongodb /var/lib/mongodb
sudo chmod 750 /var/lib/mongodb

# Restart MongoDB
sudo systemctl restart mongod

# Check status
sudo systemctl status mongod

# View logs
sudo tail -f /var/log/mongodb/mongod.log
```

### Creating Users

```bash
# Connect to MongoDB
mongosh

# Switch to admin database
use admin

# Create admin user
db.createUser({
  user: "admin",
  pwd: "YOUR_SECURE_PASSWORD_HERE",
  roles: [
    { role: "root", db: "admin" }
  ]
})

# Switch to MongoTron database
use mongotron

# Create MongoTron application user
db.createUser({
  user: "mongotron",
  pwd: "YOUR_APP_PASSWORD_HERE",
  roles: [
    { role: "readWrite", db: "mongotron" },
    { role: "dbAdmin", db: "mongotron" }
  ]
})

# Create read-only user for monitoring
db.createUser({
  user: "monitoring",
  pwd: "YOUR_MONITORING_PASSWORD_HERE",
  roles: [
    { role: "read", db: "mongotron" },
    { role: "clusterMonitor", db: "admin" }
  ]
})

# Exit
exit
```

### MongoDB Indexes

Create indexes for MongoTron collections:

```javascript
// Connect as mongotron user
mongosh -u mongotron -p YOUR_APP_PASSWORD_HERE mongotron

// Addresses collection indexes
db.addresses.createIndex({ "address": 1 }, { unique: true })
db.addresses.createIndex({ "network": 1, "address": 1 })
db.addresses.createIndex({ "subscriptionId": 1 })
db.addresses.createIndex({ "createdAt": 1 })
db.addresses.createIndex({ "lastActivity": -1 })
db.addresses.createIndex({ "active": 1, "lastActivity": -1 })

// Transactions collection indexes
db.transactions.createIndex({ "txHash": 1 }, { unique: true })
db.transactions.createIndex({ "fromAddress": 1, "timestamp": -1 })
db.transactions.createIndex({ "toAddress": 1, "timestamp": -1 })
db.transactions.createIndex({ "timestamp": -1 })
db.transactions.createIndex({ "blockNumber": -1 })
db.transactions.createIndex({ "network": 1, "timestamp": -1 })
db.transactions.createIndex({ "addresses": 1, "timestamp": -1 })

// Events collection indexes
db.events.createIndex({ "eventId": 1 }, { unique: true })
db.events.createIndex({ "address": 1, "timestamp": -1 })
db.events.createIndex({ "type": 1, "timestamp": -1 })
db.events.createIndex({ "timestamp": -1 })
db.events.createIndex({ "processed": 1, "timestamp": 1 })
db.events.createIndex({ "subscriptionId": 1, "timestamp": -1 })

// Webhooks collection indexes
db.webhooks.createIndex({ "url": 1 })
db.webhooks.createIndex({ "subscriptionId": 1 })
db.webhooks.createIndex({ "status": 1, "nextRetry": 1 })
db.webhooks.createIndex({ "createdAt": -1 })

// TTL index for old events (optional - auto-delete after 30 days)
db.events.createIndex(
  { "timestamp": 1 },
  { expireAfterSeconds: 2592000 }
)

// Compound indexes for complex queries
db.transactions.createIndex({
  "network": 1,
  "fromAddress": 1,
  "timestamp": -1
})
db.transactions.createIndex({
  "network": 1,
  "toAddress": 1,
  "timestamp": -1
})

// Text index for searching (optional)
db.transactions.createIndex({
  "txHash": "text",
  "fromAddress": "text",
  "toAddress": "text"
})
```

---

## Network Configuration

### Firewall Rules

#### UFW (Ubuntu/Debian)

```bash
# Enable UFW
sudo ufw enable

# Allow SSH
sudo ufw allow 22/tcp

# Allow MongoDB (only from specific IPs)
sudo ufw allow from 192.168.1.0/24 to any port 27017

# Allow MongoTron application
sudo ufw allow 8080/tcp

# Allow Prometheus metrics
sudo ufw allow 9090/tcp

# Check status
sudo ufw status verbose
```

#### Firewalld (RHEL/CentOS/Rocky)

```bash
# Start firewalld
sudo systemctl start firewalld
sudo systemctl enable firewalld

# Allow MongoDB from specific network
sudo firewall-cmd --permanent --add-rich-rule='
  rule family="ipv4" source address="192.168.1.0/24" port protocol="tcp" port="27017" accept'

# Allow application ports
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp

# Reload
sudo firewall-cmd --reload

# Verify
sudo firewall-cmd --list-all
```

### Network Tuning

Optimize network stack for high-throughput applications:

```bash
# Add to /etc/sysctl.conf
sudo tee -a /etc/sysctl.conf << 'EOF'

# Network Performance Tuning for MongoTron

# Increase maximum connections
net.core.somaxconn = 65535

# Increase network buffer sizes
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 67108864
net.ipv4.tcp_wmem = 4096 65536 67108864

# Enable TCP window scaling
net.ipv4.tcp_window_scaling = 1

# Increase max open files
fs.file-max = 2097152

# Reduce TIME_WAIT sockets
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_max_tw_buckets = 2000000
net.ipv4.tcp_tw_reuse = 1

# Increase local port range
net.ipv4.ip_local_port_range = 10000 65535

# Enable TCP Fast Open
net.ipv4.tcp_fastopen = 3

EOF

# Apply settings
sudo sysctl -p
```

---

## Security Hardening

### MongoDB Security

#### Enable TLS/SSL

```bash
# Generate self-signed certificate (for testing)
sudo openssl req -newkey rsa:2048 -nodes -keyout /etc/ssl/mongodb.key \
  -x509 -days 365 -out /etc/ssl/mongodb.crt

# Combine into single PEM file
sudo cat /etc/ssl/mongodb.key /etc/ssl/mongodb.crt | \
  sudo tee /etc/ssl/mongodb.pem

# Set permissions
sudo chmod 600 /etc/ssl/mongodb.pem
sudo chown mongodb:mongodb /etc/ssl/mongodb.pem

# Update mongod.conf
security:
  authorization: enabled
  tls:
    mode: requireTLS
    certificateKeyFile: /etc/ssl/mongodb.pem
    allowConnectionsWithoutCertificates: true

# Restart MongoDB
sudo systemctl restart mongod
```

#### IP Whitelisting

Update `/etc/mongod.conf`:

```yaml
net:
  port: 27017
  bindIp: 127.0.0.1,192.168.1.100  # Specific IPs only
  ipv6: false
```

#### Authentication

```bash
# Always enable authentication in production
security:
  authorization: enabled

# Use strong passwords (20+ characters, mixed case, numbers, symbols)
# Rotate passwords regularly
# Use different passwords for each user
# Store passwords securely (e.g., HashiCorp Vault)
```

### System Security

#### Disable Transparent Huge Pages (THP)

MongoDB recommends disabling THP:

```bash
# Create systemd service to disable THP at boot
sudo tee /etc/systemd/system/disable-thp.service << 'EOF'
[Unit]
Description=Disable Transparent Huge Pages (THP)
DefaultDependencies=no
After=sysinit.target local-fs.target
Before=mongod.service

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'echo never > /sys/kernel/mm/transparent_hugepage/enabled'
ExecStart=/bin/sh -c 'echo never > /sys/kernel/mm/transparent_hugepage/defrag'

[Install]
WantedBy=basic.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable disable-thp
sudo systemctl start disable-thp

# Verify
cat /sys/kernel/mm/transparent_hugepage/enabled
# Should show: always madvise [never]
```

#### Set Ulimits

```bash
# Add to /etc/security/limits.conf
sudo tee -a /etc/security/limits.conf << 'EOF'
# MongoDB ulimits
mongodb soft nofile 64000
mongodb hard nofile 64000
mongodb soft nproc 64000
mongodb hard nproc 64000
EOF

# Verify (after restarting MongoDB)
cat /proc/$(pgrep mongod)/limits
```

#### SELinux/AppArmor

If using SELinux/AppArmor, create appropriate policies or set to permissive mode for MongoDB:

```bash
# SELinux - set MongoDB to permissive
sudo semanage permissive -a mongod_t

# AppArmor - disable for MongoDB
sudo ln -s /etc/apparmor.d/usr.bin.mongod /etc/apparmor.d/disable/
sudo apparmor_parser -R /etc/apparmor.d/usr.bin.mongod
```

---

## Performance Tuning

### MongoDB Performance

#### WiredTiger Cache Size

```yaml
# Set to 50-60% of available RAM
storage:
  wiredTiger:
    engineConfig:
      cacheSizeGB: 32  # For a 64GB system
```

#### Read/Write Concerns

For MongoTron (prioritize consistency):

```javascript
// Write concern - wait for journal
db.addresses.insertOne(
  { address: "TAddr123..." },
  { writeConcern: { w: 1, j: true } }
)

// Read concern - majority (for consistency)
db.transactions.find().readConcern("majority")
```

#### Connection Pooling

```yaml
# In mongod.conf
net:
  maxIncomingConnections: 10000

# In MongoTron app (Go driver)
clientOptions := options.Client().
  SetMaxPoolSize(100).
  SetMinPoolSize(10).
  SetMaxConnIdleTime(300 * time.Second)
```

### System Performance

#### CPU Governor

```bash
# Set CPU to performance mode
for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
  echo performance | sudo tee $cpu
done

# Make permanent (add to /etc/rc.local or systemd service)
```

#### NUMA Optimization

```bash
# Check if NUMA is enabled
numactl --hardware

# Run MongoDB with NUMA interleave (recommended)
# Update /usr/lib/systemd/system/mongod.service
ExecStart=numactl --interleave=all /usr/bin/mongod --config /etc/mongod.conf

# Reload systemd
sudo systemctl daemon-reload
sudo systemctl restart mongod
```

#### Disk I/O Scheduler

```bash
# For NVMe SSDs, use 'none' or 'mq-deadline'
echo none | sudo tee /sys/block/nvme0n1/queue/scheduler

# Make permanent (add to /etc/udev/rules.d/60-scheduler.rules)
echo 'ACTION=="add|change", KERNEL=="nvme[0-9]n[0-9]", ATTR{queue/scheduler}="none"' | \
  sudo tee /etc/udev/rules.d/60-scheduler.rules
```

### Monitoring Performance

```bash
# MongoDB stats
mongosh -u admin -p PASSWORD --authenticationDatabase admin << 'EOF'
db.serverStatus()
db.currentOp()
db.stats()
EOF

# System stats
iostat -xz 1
vmstat 1
sar -u 1
sar -r 1

# ZFS stats
zpool iostat -v mongopool 1
cat /proc/spl/kstat/zfs/arcstats
```

---

## Monitoring & Alerting

### Prometheus Integration

#### MongoDB Exporter

```bash
# Install MongoDB exporter
wget https://github.com/percona/mongodb_exporter/releases/download/v0.40.0/mongodb_exporter-0.40.0.linux-amd64.tar.gz
tar xvf mongodb_exporter-0.40.0.linux-amd64.tar.gz
sudo mv mongodb_exporter /usr/local/bin/

# Create systemd service
sudo tee /etc/systemd/system/mongodb-exporter.service << 'EOF'
[Unit]
Description=MongoDB Exporter
After=network.target

[Service]
Type=simple
User=mongodb
Environment="MONGODB_URI=mongodb://monitoring:PASSWORD@localhost:27017"
ExecStart=/usr/local/bin/mongodb_exporter

[Install]
WantedBy=multi-user.target
EOF

# Start exporter
sudo systemctl daemon-reload
sudo systemctl start mongodb-exporter
sudo systemctl enable mongodb-exporter

# Verify (should return metrics)
curl http://localhost:9216/metrics
```

#### ZFS Monitoring

```bash
# Install ZFS exporter
sudo apt install -y prometheus-zfs-exporter

# Or use node_exporter with ZFS metrics
wget https://github.com/prometheus/node_exporter/releases/download/v1.7.0/node_exporter-1.7.0.linux-amd64.tar.gz
tar xvf node_exporter-1.7.0.linux-amd64.tar.gz
sudo mv node_exporter-1.7.0.linux-amd64/node_exporter /usr/local/bin/

# Create systemd service
sudo tee /etc/systemd/system/node-exporter.service << 'EOF'
[Unit]
Description=Node Exporter
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/node_exporter \
  --collector.zfs \
  --collector.filesystem \
  --collector.diskstats

[Install]
WantedBy=multi-user.target
EOF

# Start exporter
sudo systemctl daemon-reload
sudo systemctl start node-exporter
sudo systemctl enable node-exporter
```

### Grafana Dashboards

Import these dashboard IDs in Grafana:

- **MongoDB**: 2583 (MongoDB Overview)
- **System**: 1860 (Node Exporter Full)
- **ZFS**: 13465 (ZFS Dashboard)

### Alerting Rules

Example Prometheus alerting rules:

```yaml
# /etc/prometheus/rules/mongotron.yml
groups:
  - name: mongotron
    interval: 30s
    rules:
      - alert: MongoDBDown
        expr: mongodb_up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "MongoDB is down"
          
      - alert: MongoDBHighConnections
        expr: mongodb_connections{state="current"} > 8000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MongoDB connection count high"
          
      - alert: ZFSPoolDegraded
        expr: node_zfs_zpool_state{state="degraded"} > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "ZFS pool is degraded"
          
      - alert: DiskSpaceLow
        expr: (node_filesystem_avail_bytes{mountpoint="/var/lib/mongodb"} / 
               node_filesystem_size_bytes{mountpoint="/var/lib/mongodb"}) < 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MongoDB disk space below 10%"
```

---

## Backup & Recovery

### ZFS Snapshots

```bash
# Create snapshot before major operations
sudo zfs snapshot mongopool/mongodb@pre-upgrade-$(date +%Y%m%d)

# Automated snapshots (every 4 hours)
sudo tee /etc/cron.d/zfs-snapshots << 'EOF'
0 */4 * * * root /usr/sbin/zfs snapshot mongopool/mongodb@auto-$(date +\%Y\%m\%d-\%H\%M)
EOF

# Cleanup old snapshots (keep last 7 days)
sudo tee /etc/cron.daily/zfs-snapshot-cleanup << 'EOF'
#!/bin/bash
CUTOFF=$(date -d '7 days ago' +%Y%m%d)
zfs list -H -t snapshot -o name | grep "mongopool/mongodb@auto-" | while read snap; do
  SNAP_DATE=$(echo $snap | grep -oP '\d{8}')
  if [[ $SNAP_DATE < $CUTOFF ]]; then
    zfs destroy $snap
  fi
done
EOF
sudo chmod +x /etc/cron.daily/zfs-snapshot-cleanup
```

### MongoDB Backups

```bash
# Full backup with mongodump
mongodump --uri="mongodb://admin:PASSWORD@localhost:27017" \
  --out=/backup/mongodump-$(date +%Y%m%d-%H%M%S) \
  --gzip

# Incremental backup using oplog
mongodump --uri="mongodb://admin:PASSWORD@localhost:27017" \
  --oplog \
  --out=/backup/incremental-$(date +%Y%m%d-%H%M%S) \
  --gzip

# Automated daily backups
sudo tee /etc/cron.daily/mongodb-backup << 'EOF'
#!/bin/bash
BACKUP_DIR=/backup/mongodb/$(date +%Y%m%d)
mkdir -p $BACKUP_DIR
mongodump --uri="mongodb://admin:PASSWORD@localhost:27017" \
  --out=$BACKUP_DIR --gzip
find /backup/mongodb -type d -mtime +7 -exec rm -rf {} \;
EOF
sudo chmod +x /etc/cron.daily/mongodb-backup
```

### Restore Procedures

```bash
# Restore from ZFS snapshot
sudo zfs rollback mongopool/mongodb@pre-upgrade-20251005

# Restore from mongodump
sudo systemctl stop mongod
sudo rm -rf /var/lib/mongodb/*
mongorestore --uri="mongodb://admin:PASSWORD@localhost:27017" \
  --gzip /backup/mongodump-20251005-120000
sudo systemctl start mongod

# Point-in-time recovery using oplog
mongorestore --uri="mongodb://admin:PASSWORD@localhost:27017" \
  --gzip --oplogReplay /backup/incremental-20251005-120000
```

---

## Scaling Strategies

### Vertical Scaling (Scale Up)

1. **Increase CPU**: More cores = more concurrent operations
2. **Increase RAM**: Larger WiredTiger cache = better performance
3. **Faster Storage**: NVMe > SSD > HDD
4. **Network**: 10 Gbps+ for high-throughput

### Horizontal Scaling (Scale Out)

#### MongoDB Replica Set

```javascript
// Initialize replica set
rs.initiate({
  _id: "mongotron-rs",
  members: [
    { _id: 0, host: "mongo1.example.com:27017", priority: 2 },
    { _id: 1, host: "mongo2.example.com:27017", priority: 1 },
    { _id: 2, host: "mongo3.example.com:27017", priority: 1, arbiterOnly: true }
  ]
})

// Add member
rs.add("mongo4.example.com:27017")

// Check status
rs.status()
```

#### MongoDB Sharding

For deployments monitoring >100K addresses:

```javascript
// Enable sharding for database
sh.enableSharding("mongotron")

// Shard collections by address
sh.shardCollection("mongotron.transactions", { "fromAddress": "hashed" })
sh.shardCollection("mongotron.events", { "address": "hashed" })

// Check shard status
sh.status()
```

#### Application Scaling

```bash
# Run multiple MongoTron instances behind load balancer
# Instance 1
MONGOTRON_PORT=8080 ./mongotron

# Instance 2
MONGOTRON_PORT=8081 ./mongotron

# NGINX load balancer config
upstream mongotron {
  least_conn;
  server 127.0.0.1:8080;
  server 127.0.0.1:8081;
  server 127.0.0.1:8082;
}
```

---

## Troubleshooting

### Common Issues

#### MongoDB Won't Start

```bash
# Check logs
sudo tail -f /var/log/mongodb/mongod.log

# Common causes:
# 1. Port already in use
sudo lsof -i :27017

# 2. Permission issues
sudo chown -R mongodb:mongodb /var/lib/mongodb
sudo chmod 750 /var/lib/mongodb

# 3. Lock file issues
sudo rm /var/lib/mongodb/mongod.lock
sudo systemctl restart mongod
```

#### ZFS Pool Degraded

```bash
# Check pool status
sudo zpool status mongopool

# Replace failed drive
sudo zpool replace mongopool /dev/nvme0n1 /dev/nvme2n1

# Scrub pool after replacement
sudo zpool scrub mongopool
```

#### High Memory Usage

```bash
# Check MongoDB cache
mongosh -u admin -p PASSWORD --eval "db.serverStatus().wiredTiger.cache"

# Check ZFS ARC
cat /proc/spl/kstat/zfs/arcstats | grep "^c "

# Reduce ZFS ARC if needed
echo 4294967296 | sudo tee /sys/module/zfs/parameters/zfs_arc_max
```

#### Slow Queries

```bash
# Enable profiling
mongosh -u admin -p PASSWORD << 'EOF'
use mongotron
db.setProfilingLevel(2)
EOF

# Check slow queries
mongosh -u admin -p PASSWORD << 'EOF'
use mongotron
db.system.profile.find().sort({ts:-1}).limit(10).pretty()
EOF

# Check missing indexes
mongosh -u admin -p PASSWORD << 'EOF'
use mongotron
db.transactions.find({fromAddress: "TAddr123..."}).explain("executionStats")
EOF
```

### Performance Issues

```bash
# Check system load
uptime
htop

# Check I/O wait
iostat -x 1

# Check ZFS performance
zpool iostat -lpv mongopool 5

# Check MongoDB performance
mongosh -u admin -p PASSWORD << 'EOF'
db.serverStatus().metrics.operation
db.serverStatus().connections
EOF
```

### Diagnostic Commands

```bash
# Full system diagnostics script
sudo tee /usr/local/bin/mongotron-diag << 'EOF'
#!/bin/bash
echo "=== System Info ==="
uname -a
uptime
free -h

echo -e "\n=== MongoDB Status ==="
systemctl status mongod

echo -e "\n=== ZFS Pool Status ==="
zpool status mongopool
zpool list mongopool

echo -e "\n=== Disk Usage ==="
df -h /var/lib/mongodb

echo -e "\n=== MongoDB Connections ==="
mongosh -u monitoring -p PASSWORD --eval "db.serverStatus().connections"

echo -e "\n=== Network Connections ==="
ss -tnp | grep :27017 | wc -l

echo -e "\n=== Top Processes ==="
ps aux --sort=-%mem | head -10
EOF
sudo chmod +x /usr/local/bin/mongotron-diag
```

---

## Example Production Setup

### Complete Setup Script

```bash
#!/bin/bash
# Production MongoTron Infrastructure Setup

set -e

echo "=== Installing ZFS ==="
sudo apt update
sudo apt install -y zfsutils-linux

echo "=== Creating ZFS Pool ==="
sudo zpool create -f \
  -o ashift=12 \
  -O compression=lz4 \
  -O atime=off \
  -O recordsize=16K \
  -O logbias=latency \
  mongopool /dev/nvme0n1

echo "=== Creating MongoDB Dataset ==="
sudo zfs create \
  -o mountpoint=/var/lib/mongodb \
  -o recordsize=16K \
  -o primarycache=metadata \
  mongopool/mongodb

echo "=== Installing MongoDB ==="
curl -fsSL https://www.mongodb.org/static/pgp/server-7.0.asc | \
  sudo gpg -o /usr/share/keyrings/mongodb-server-7.0.gpg --dearmor

echo "deb [ arch=amd64,arm64 signed-by=/usr/share/keyrings/mongodb-server-7.0.gpg ] \
  https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/7.0 multiverse" | \
  sudo tee /etc/apt/sources.list.d/mongodb-org-7.0.list

sudo apt update
sudo apt install -y mongodb-org

echo "=== Configuring MongoDB ==="
sudo chown -R mongodb:mongodb /var/lib/mongodb
sudo chmod 750 /var/lib/mongodb

echo "=== Starting MongoDB ==="
sudo systemctl start mongod
sudo systemctl enable mongod

echo "=== Setup Complete ==="
echo "MongoDB connection: mongodb://localhost:27017"
echo "Create users with: mongosh"
```

---

## References

- [MongoDB Production Notes](https://docs.mongodb.com/manual/administration/production-notes/)
- [ZFS Best Practices](https://www.truenas.com/docs/references/zfsprimer/)
- [MongoTron GitHub](https://github.com/frstrtr/mongotron)
- [Prometheus MongoDB Exporter](https://github.com/percona/mongodb_exporter)

---

**Last Updated**: October 5, 2025  
**Version**: 1.0.0  
**Tested On**: Ubuntu 24.04 LTS, MongoDB 7.0.25, ZFS 2.1.x
