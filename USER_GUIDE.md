# DTVN - Distributed Ticket Validation Network: Complete User Guide

## Table of Contents

1. [Introduction](#introduction)
2. [Prerequisites](#prerequisites)
3. [Quick Start Guide](#quick-start-guide)
4. [Architecture Overview](#architecture-overview)
5. [Installation and Setup](#installation-and-setup)
6. [Configuration Guide](#configuration-guide)
7. [Running the System](#running-the-system)
8. [API Reference](#api-reference)
9. [Component Details](#component-details)
10. [Testing and Simulation](#testing-and-simulation)
11. [Monitoring and Metrics](#monitoring-and-metrics)
12. [Troubleshooting](#troubleshooting)
13. [What to Change vs. What to Keep](#what-to-change-vs-what-to-keep)

---

## Introduction

**DTVN (Distributed Ticket Validation Network)** is a Byzantine fault-tolerant distributed system designed for ticket validation across multiple nodes. It implements:

- **PBFT (Practical Byzantine Fault Tolerance)** consensus protocol
- **P2P networking** using libp2p
- **Gossip protocol** for efficient message dissemination
- **Vector clocks** for causality tracking
- **Persistent storage** using BoltDB
- **REST API** and WebSocket support for real-time updates

### What This System Does

The system allows multiple validator nodes to collaboratively validate, consume, and dispute tickets in a distributed manner while tolerating Byzantine (malicious or faulty) nodes. It can tolerate up to `f` faulty nodes where `f < n/3` (n = total nodes).

### Use Cases

- Event ticket validation systems
- Multi-party verification systems
- Distributed ledger applications
- Any system requiring Byzantine fault tolerance

---

## Prerequisites

### Required Software

1. **Go 1.25.5 or higher**
   - Download from: https://golang.org/dl/
   - Verify installation: `go version`

2. **Git**
   - Verify installation: `git --version`

3. **Make**
   - Linux/macOS: Usually pre-installed
   - Windows: Install via MinGW or WSL

### Optional Software

4. **Docker** (for containerized deployment)
   - Download from: https://www.docker.com/get-started
   - Verify installation: `docker --version`

5. **Docker Compose** (for multi-node networks)
   - Usually included with Docker Desktop
   - Verify installation: `docker-compose --version`

6. **Protocol Buffers Compiler** (for development only)
   - Only needed if modifying `.proto` files
   - Install: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`

### System Requirements

- **Disk Space**: Minimum 500MB for binaries and dependencies
- **RAM**: Minimum 512MB per validator node
- **Network**: Open ports for P2P communication (default: 4001) and API (default: 8080)

---

## Quick Start Guide

### 1. Clone the Repository

```bash
git clone https://github.com/Hetti219/distributed-ticket-validation.git
cd distributed-ticket-validation
```

### 2. Install Dependencies

```bash
make deps
# or manually:
go mod download
```

### 3. Build the Binaries

```bash
make build
```

This creates two binaries in the `bin/` directory:
- `bin/validator` - Main validator node
- `bin/simulator` - Network simulator for testing

### 4. Run a Single Validator Node

```bash
make run-validator
```

This starts a primary validator node on:
- P2P port: 4001
- API port: 8080
- Metrics port: 9090

### 5. Test the API

```bash
# Validate a ticket
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001", "data": "event-data"}'

# Check ticket status
curl http://localhost:8080/api/v1/tickets/TICKET-001

# Get node statistics
curl http://localhost:8080/api/v1/stats
```

---

## Architecture Overview

### System Layers

```
┌─────────────────────────────────────────┐
│         Layer 5: REST API               │  ← User Interface
├─────────────────────────────────────────┤
│         Layer 4: Storage (BoltDB)       │  ← Persistence
├─────────────────────────────────────────┤
│    Layer 3: State Machine + Clocks     │  ← Business Logic
├─────────────────────────────────────────┤
│       Layer 2: PBFT Consensus           │  ← Byzantine Tolerance
├─────────────────────────────────────────┤
│       Layer 1: Gossip Protocol          │  ← Message Propagation
├─────────────────────────────────────────┤
│    Layer 0.5: Peer Discovery (DHT)      │  ← Service Discovery
├─────────────────────────────────────────┤
│      Layer 0: P2P Network (libp2p)      │  ← Network Transport
└─────────────────────────────────────────┘
```

### Key Components

1. **P2P Network** (`pkg/network/`)
   - Handles peer-to-peer communication using libp2p
   - Supports multiple transports: TCP, QUIC, WebSocket
   - Provides NAT traversal and security (TLS, Noise)

2. **Peer Discovery** (`pkg/network/discovery.go`)
   - Kademlia DHT for automatic peer discovery
   - Bootstrap peer support for network joining

3. **Gossip Engine** (`pkg/gossip/`)
   - Epidemic broadcast protocol
   - Message deduplication and caching
   - Anti-entropy reconciliation

4. **PBFT Consensus** (`pkg/consensus/`)
   - Three-phase commit: PRE-PREPARE → PREPARE → COMMIT
   - View change mechanism for leader election
   - Tolerates up to f < n/3 Byzantine nodes

5. **State Machine** (`pkg/state/`)
   - Manages ticket states: ISSUED → PENDING → VALIDATED → CONSUMED/DISPUTED
   - Vector clock integration for causality

6. **Storage** (`pkg/storage/`)
   - Embedded BoltDB key-value store
   - Atomic transactions and backup support

7. **API Server** (`pkg/api/`)
   - RESTful HTTP endpoints
   - WebSocket for real-time updates
   - Prometheus metrics endpoint

---

## Installation and Setup

### Development Setup

```bash
# Clone the repository
git clone https://github.com/Hetti219/distributed-ticket-validation.git
cd distributed-ticket-validation

# Run development setup
make dev-setup
```

This command:
- Downloads all Go dependencies
- Installs protocol buffer tools
- Creates necessary directories (`data/`, `bin/`)

### Building from Source

```bash
# Build both validator and simulator
make build

# Build only validator
go build -o bin/validator cmd/validator/main.go

# Build only simulator
go build -o bin/simulator cmd/simulator/main.go
```

### Docker Setup

```bash
# Build Docker image
make docker-build

# Run single validator in Docker
make docker-run
```

### Multi-Node Network Setup

#### Option 1: Using Makefile (4 nodes)

```bash
make run-network
```

This starts 4 validator nodes:
- Node 0 (Primary): ports 4001 (P2P), 8081 (API)
- Node 1: ports 4002 (P2P), 8082 (API)
- Node 2: ports 4003 (P2P), 8083 (API)
- Node 3: ports 4004 (P2P), 8084 (API)

#### Option 2: Using Docker Compose (7 nodes)

```bash
make docker-compose-up
```

This starts a complete network with:
- 7 validator nodes
- Prometheus monitoring (port 9999)
- Grafana dashboards (port 3000)

To stop:
```bash
make docker-compose-down
```

---

## Configuration Guide

### Command-Line Flags

The validator binary accepts the following flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-id` | string | `node0` | Unique node identifier |
| `-port` | int | `4001` | P2P listen port |
| `-api-port` | int | `8080` | HTTP API server port |
| `-data-dir` | string | `./data` | Directory for persistent data |
| `-bootstrap` | string | `""` | Comma-separated bootstrap peer addresses |
| `-bootstrap-node` | bool | `false` | Run as bootstrap node |
| `-primary` | bool | `false` | Run as primary (leader) node |
| `-total-nodes` | int | `4` | Total number of validators in network |

### Example Commands

**Run as Primary Node:**
```bash
./bin/validator -id node0 -port 4001 -api-port 8080 -primary -total-nodes 7
```

**Run as Replica Node:**
```bash
./bin/validator -id node1 -port 4002 -api-port 8081 -total-nodes 7 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/<PEER_ID>"
```

**Run as Bootstrap Node:**
```bash
./bin/validator -id bootstrap -port 4000 -api-port 8000 -bootstrap-node -primary -total-nodes 7
```

### Configuration File (validator.yaml)

Location: `config/validator.yaml`

```yaml
node:
  id: "validator-1"                    # Unique node identifier
  data_dir: "./data/validator-1"      # Data storage directory

network:
  listen_port: 4001                    # P2P listen port
  bootstrap_mode: false                # Is this a bootstrap node?
  bootstrap_peers:                     # List of bootstrap peers
    - "/ip4/127.0.0.1/tcp/4000/p2p/12D3KooWBootstrapPeerID"

api:
  address: "0.0.0.0"                   # API bind address
  port: 8080                           # API port
  enable_cors: true                    # Enable CORS

consensus:
  is_primary: false                    # Is this the primary node?
  total_nodes: 7                       # Total validators
  view_timeout: 5s                     # Timeout for view change
  checkpoint_interval: 100             # Checkpoint every N requests

gossip:
  fanout: 3                            # Peers to gossip to
  ttl: 10                              # Message TTL
  cache_size: 10000                    # Message cache size
  anti_entropy_interval: 5s            # Reconciliation interval

storage:
  type: "boltdb"                       # Database type
  path: "./data/validator-1/db"       # Database file path
  timeout: 10s                         # Transaction timeout

metrics:
  enabled: true                        # Enable Prometheus metrics
  address: "0.0.0.0"                  # Metrics bind address
  port: 9090                           # Metrics port

logging:
  level: "info"                        # Log level: debug, info, warn, error
  format: "json"                       # Format: json or text
  output: "stdout"                     # Output: stdout or file path
```

---

## Running the System

### Single Node Deployment

**Step 1: Start the validator**
```bash
./bin/validator -id node0 -port 4001 -api-port 8080 -primary -total-nodes 1
```

**Step 2: Verify it's running**
```bash
curl http://localhost:8080/health
```

Expected response: `{"status":"healthy"}`

### Multi-Node Deployment (Manual)

**Step 1: Start the primary node**
```bash
./bin/validator -id node0 -port 4001 -api-port 8080 -primary -total-nodes 4 -data-dir ./data/node0
```

**Step 2: Wait 2 seconds, then start replica nodes**
```bash
# Node 1
./bin/validator -id node1 -port 4002 -api-port 8081 -total-nodes 4 -data-dir ./data/node1 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001"

# Node 2
./bin/validator -id node2 -port 4003 -api-port 8082 -total-nodes 4 -data-dir ./data/node2 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001"

# Node 3
./bin/validator -id node3 -port 4004 -api-port 8083 -total-nodes 4 -data-dir ./data/node3 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001"
```

### Docker Compose Deployment

**Start the network:**
```bash
docker-compose up -d
```

**Check status:**
```bash
docker-compose ps
```

**View logs:**
```bash
# All nodes
docker-compose logs -f

# Specific node
docker-compose logs -f validator-0
```

**Stop the network:**
```bash
docker-compose down
```

### Graceful Shutdown

The validator handles SIGINT (Ctrl+C) and SIGTERM gracefully:
1. Stops accepting new requests
2. Completes ongoing consensus rounds
3. Flushes data to disk
4. Closes network connections
5. Shuts down cleanly

---

## API Reference

### Base URL

```
http://localhost:8080/api/v1
```

### Endpoints

#### 1. Validate Ticket

**Request:**
```bash
POST /api/v1/tickets/validate
Content-Type: application/json

{
  "ticket_id": "TICKET-001",
  "data": "event-metadata-or-hash"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Ticket validated successfully",
  "ticket_id": "TICKET-001"
}
```

#### 2. Consume Ticket

**Request:**
```bash
POST /api/v1/tickets/consume
Content-Type: application/json

{
  "ticket_id": "TICKET-001"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Ticket consumed successfully",
  "ticket_id": "TICKET-001"
}
```

#### 3. Dispute Ticket

**Request:**
```bash
POST /api/v1/tickets/dispute
Content-Type: application/json

{
  "ticket_id": "TICKET-001"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Ticket disputed successfully",
  "ticket_id": "TICKET-001"
}
```

#### 4. Get Ticket Status

**Request:**
```bash
GET /api/v1/tickets/{ticket_id}
```

**Response:**
```json
{
  "id": "TICKET-001",
  "state": "VALIDATED",
  "data": "event-metadata-or-hash",
  "validator_id": "node0",
  "timestamp": 1733097600,
  "vector_clock": {
    "node0": 5,
    "node1": 3,
    "node2": 4
  },
  "metadata": {}
}
```

#### 5. List All Tickets

**Request:**
```bash
GET /api/v1/tickets
```

**Response:**
```json
{
  "tickets": [
    {
      "id": "TICKET-001",
      "state": "VALIDATED",
      "validator_id": "node0"
    },
    {
      "id": "TICKET-002",
      "state": "CONSUMED",
      "validator_id": "node1"
    }
  ],
  "count": 2
}
```

#### 6. Node Status

**Request:**
```bash
GET /api/v1/status
```

**Response:**
```json
{
  "node_id": "node0",
  "role": "PRIMARY",
  "is_healthy": true,
  "peer_count": 6,
  "current_view": 0,
  "sequence_number": 42,
  "uptime_seconds": 3600
}
```

#### 7. Node Statistics

**Request:**
```bash
GET /api/v1/stats
```

**Response:**
```json
{
  "node_id": "node0",
  "peer_count": 6,
  "is_primary": true,
  "current_view": 0,
  "sequence": 42,
  "cache_size": 150,
  "storage_stats": {
    "tickets": 25,
    "consensus_logs": 42,
    "checkpoints": 0
  }
}
```

#### 8. Health Check

**Request:**
```bash
GET /health
```

**Response:**
```json
{
  "status": "healthy"
}
```

#### 9. Metrics (Prometheus)

**Request:**
```bash
GET /metrics
```

**Response:** (Prometheus text format)
```
# HELP consensus_rounds_total Total number of consensus rounds
# TYPE consensus_rounds_total counter
consensus_rounds_total 42

# HELP gossip_messages_sent_total Total gossip messages sent
# TYPE gossip_messages_sent_total counter
gossip_messages_sent_total 1250
...
```

### WebSocket Endpoint

**Connect:**
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('Connected to validator');
};

ws.onmessage = (event) => {
  const update = JSON.parse(event.data);
  console.log('Ticket update:', update);
};
```

**Message Format:**
```json
{
  "type": "ticket_update",
  "ticket_id": "TICKET-001",
  "old_state": "PENDING",
  "new_state": "VALIDATED",
  "timestamp": 1733097600
}
```

---

## Component Details

### 1. P2P Network (`pkg/network/`)

**Files:**
- `host.go` - Main P2P host implementation
- `discovery.go` - Peer discovery using Kademlia DHT

**Key Features:**
- Multiple transport protocols (TCP, QUIC, WebSocket)
- Automatic NAT traversal (UPnP, NAT-PMP)
- Secure communication (TLS, Noise protocol)
- Stream multiplexing (yamux)
- Protocol ID: `/validator/1.0.0`

**Configuration:**
```go
config := &network.Config{
    ListenPort:    4001,
    BootstrapMode: false,
    DataDir:       "./data",
}
```

### 2. Gossip Protocol (`pkg/gossip/`)

**Files:**
- `engine.go` - Gossip engine
- `cache.go` - Message cache

**Key Features:**
- Epidemic broadcast (push-based)
- Dynamic fanout: √n peers
- Message deduplication via bloom filters
- TTL-based propagation (default: 10 hops)
- Anti-entropy reconciliation (every 5s)
- LRU cache (10,000 messages)

**Configuration:**
```go
config := &gossip.Config{
    NodeID: "node0",
    Fanout: 3,           // 0 = auto-calculate
    TTL:    10,
    CacheSize: 10000,
}
```

### 3. PBFT Consensus (`pkg/consensus/`)

**Files:**
- `pbft.go` - PBFT implementation
- `quorum.go` - Quorum verification

**Three-Phase Protocol:**
1. **PRE-PREPARE**: Primary proposes request with sequence number
2. **PREPARE**: Replicas agree on ordering (requires 2f+1 messages)
3. **COMMIT**: Replicas commit (requires 2f+1 messages)

**Byzantine Tolerance:**
- Tolerates `f < n/3` Byzantine nodes
- Example: With 7 nodes, tolerates up to 2 Byzantine nodes

**View Change:**
- Triggers on primary failure or timeout (default: 5s)
- Elects new primary: `(view + 1) % n`

**Configuration:**
```go
config := &consensus.Config{
    NodeID:      "node0",
    TotalNodes:  7,
    IsPrimary:   true,
    ViewTimeout: 5 * time.Second,
}
```

### 4. State Machine (`pkg/state/`)

**Files:**
- `machine.go` - Ticket state machine
- `vectorclock.go` - Vector clock implementation

**Ticket States:**
- `ISSUED` → Ticket created
- `PENDING` → Validation in progress
- `VALIDATED` → Ticket validated by consensus
- `CONSUMED` → Ticket used/redeemed
- `DISPUTED` → Ticket contested

**State Transitions:**
```
ISSUED → PENDING → VALIDATED → CONSUMED
                            ↘ DISPUTED
```

**Vector Clocks:**
- Tracks causality between operations
- Detects concurrent updates
- Format: `{"node0": 5, "node1": 3, "node2": 4}`

### 5. Storage (`pkg/storage/`)

**Files:**
- `store.go` - BoltDB wrapper

**Buckets:**
- `tickets` - Ticket records
- `consensus` - Consensus logs
- `checkpoints` - PBFT checkpoints
- `metadata` - System metadata

**Operations:**
```go
// Save ticket
store.SaveTicket(&TicketRecord{
    ID:    "TICKET-001",
    State: "VALIDATED",
    Data:  []byte("..."),
})

// Get ticket
ticket, err := store.GetTicket("TICKET-001")

// Backup
err := store.Backup("./backup.db")
```

### 6. API Server (`pkg/api/`)

**Files:**
- `server.go` - HTTP server and handlers

**Features:**
- Gorilla Mux router
- CORS support (configurable)
- Graceful shutdown
- WebSocket support (Gorilla WebSocket)
- Prometheus metrics integration

---

## Testing and Simulation

### Unit Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage
# Opens coverage.html in browser
```

### Network Simulator

The simulator (`cmd/simulator/main.go`) is a comprehensive testing tool.

**Basic Usage:**
```bash
make run-simulator
```

**Custom Configuration:**
```bash
./bin/simulator \
  -nodes 10 \
  -byzantine 3 \
  -tickets 200 \
  -duration 120s \
  -latency 100ms \
  -packet-loss 0.02
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-nodes` | 7 | Number of validator nodes |
| `-byzantine` | 2 | Number of Byzantine (faulty) nodes |
| `-tickets` | 100 | Tickets to simulate |
| `-duration` | 60s | Simulation duration |
| `-latency` | 50ms | Network latency |
| `-packet-loss` | 0.01 | Packet loss rate (0.0-1.0) |
| `-partition` | false | Enable network partitions |

**With Network Partition:**
```bash
make run-simulator-partition
```

**Simulator Output:**
```
=== DTVN Network Simulator ===
Configuration:
  Nodes: 7
  Byzantine: 2
  Tickets: 100
  Duration: 60s
  Latency: 50ms
  Packet Loss: 1.0%
  Network Partition: false

Starting simulation...
[00:05] Validated 25 tickets
[00:10] Validated 50 tickets
[00:15] Byzantine node detected: node5
[00:20] Validated 75 tickets
[00:25] Validated 100 tickets

=== Simulation Results ===
Duration: 25.3s
Total Tickets: 100
Validated: 98 (98.0%)
Failed: 2 (2.0%)
Average Latency: 125ms
Consensus Success Rate: 98.0%
Byzantine Nodes Detected: 1
```

### Linting and Formatting

```bash
# Format code
make fmt

# Run linter (requires golangci-lint)
make lint
```

---

## Monitoring and Metrics

### Prometheus Setup

**1. Start Prometheus (if using Docker Compose):**
```bash
docker-compose up -d prometheus
```

**2. Access Prometheus UI:**
```
http://localhost:9999
```

**3. Configuration:**
Location: `config/prometheus.yml`

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'dtvn'
    environment: 'development'

scrape_configs:
  - job_name: 'validator-nodes'
    static_configs:
      - targets:
          - 'validator-0:9090'
          - 'validator-1:9090'
          - 'validator-2:9090'
          # ... all validators
```

### Grafana Setup

**1. Start Grafana (if using Docker Compose):**
```bash
docker-compose up -d grafana
```

**2. Access Grafana:**
```
http://localhost:3000
Username: admin
Password: admin
```

**3. Add Prometheus Data Source:**
- Go to Configuration → Data Sources
- Add Prometheus
- URL: `http://prometheus:9090`
- Click "Save & Test"

### Key Metrics

**Consensus Metrics:**
- `consensus_rounds_total` - Total consensus rounds
- `consensus_success_total` - Successful consensus rounds
- `consensus_failures_total` - Failed consensus rounds
- `consensus_latency_seconds` - Consensus round latency

**Gossip Metrics:**
- `gossip_messages_sent_total` - Messages sent
- `gossip_messages_received_total` - Messages received
- `gossip_cache_size` - Current cache size
- `gossip_cache_hits_total` - Cache hits
- `gossip_cache_misses_total` - Cache misses

**Network Metrics:**
- `network_peers_connected` - Connected peers
- `network_bytes_sent_total` - Bytes sent
- `network_bytes_received_total` - Bytes received

**Ticket Metrics:**
- `tickets_validated_total` - Validated tickets
- `tickets_consumed_total` - Consumed tickets
- `tickets_disputed_total` - Disputed tickets

**Storage Metrics:**
- `storage_operations_total` - Storage operations
- `storage_latency_seconds` - Operation latency

**Sample Queries:**

```promql
# Consensus success rate
rate(consensus_success_total[5m]) / rate(consensus_rounds_total[5m])

# Average gossip latency
rate(gossip_latency_seconds_sum[5m]) / rate(gossip_latency_seconds_count[5m])

# Peer count over time
network_peers_connected

# Ticket validation rate
rate(tickets_validated_total[1m])
```

---

## Troubleshooting

### Common Issues

#### 1. Node Won't Start

**Symptom:**
```
Failed to create validator node: failed to initialize P2P host: ...
```

**Solutions:**
- Check if the port is already in use: `lsof -i :4001` (or `netstat -ano | findstr 4001` on Windows)
- Change the port: `-port 4002`
- Ensure data directory permissions: `chmod 755 ./data`

#### 2. Nodes Can't Discover Each Other

**Symptom:**
```
Peer count: 0
```

**Solutions:**
- Verify bootstrap peer address is correct
- Check firewall settings - ensure P2P ports are open
- Wait 30 seconds for DHT to propagate
- Check logs for connection errors

#### 3. Consensus Not Making Progress

**Symptom:**
```
View timeout triggered
```

**Solutions:**
- Ensure primary node is running: Check `-primary` flag
- Verify `total-nodes` matches actual node count
- Check network connectivity between nodes
- Look for Byzantine behavior in logs

#### 4. API Returns "Connection Refused"

**Symptom:**
```
curl: (7) Failed to connect to localhost port 8080: Connection refused
```

**Solutions:**
- Verify node is running: `ps aux | grep validator`
- Check API port: `-api-port 8080`
- Ensure API server started: Look for "API server on port 8080" in logs
- Try health check: `curl http://localhost:8080/health`

#### 5. Storage Errors

**Symptom:**
```
failed to initialize storage: timeout
```

**Solutions:**
- Check disk space: `df -h`
- Verify data directory exists and is writable
- Close other processes using the database file
- Delete corrupted database: `rm ./data/node0/node0.db` (data will be lost)

#### 6. Docker Compose Containers Won't Start

**Symptom:**
```
ERROR: for validator-1  Container "xxx" is unhealthy
```

**Solutions:**
- Check logs: `docker-compose logs validator-1`
- Ensure validator-0 (bootstrap) is healthy first
- Increase wait time between node starts
- Verify Docker resources (CPU, memory)

### Debug Logging

**Enable debug logging:**

Modify `logging.level` in `config/validator.yaml`:
```yaml
logging:
  level: "debug"  # Was "info"
```

Or via command line (requires code modification):
```bash
./bin/validator -id node0 -port 4001 -api-port 8080 -primary -total-nodes 4 -log-level debug
```

### Network Diagnostics

**Check peer connections:**
```bash
curl http://localhost:8080/api/v1/stats | jq '.peer_count'
```

**Check node status:**
```bash
curl http://localhost:8080/api/v1/status | jq
```

**Monitor logs:**
```bash
# For Docker Compose
docker-compose logs -f validator-0

# For manual deployment
tail -f ./logs/validator.log
```

---

## What to Change vs. What to Keep

### ✅ SAFE TO CHANGE (Configuration & Tuning)

#### 1. Node Configuration (`config/validator.yaml`)

**Node Identity:**
```yaml
node:
  id: "validator-1"          # ✅ Change to unique identifier
  data_dir: "./data/validator-1"  # ✅ Change to preferred location
```

**Network Settings:**
```yaml
network:
  listen_port: 4001          # ✅ Change if port conflicts
  bootstrap_peers:           # ✅ Add/modify peer addresses
    - "/ip4/10.0.0.5/tcp/4000/p2p/<PEER_ID>"
```

**API Configuration:**
```yaml
api:
  address: "0.0.0.0"         # ✅ Change to specific interface
  port: 8080                 # ✅ Change if port conflicts
  enable_cors: true          # ✅ Disable for production if not needed
```

**Consensus Tuning:**
```yaml
consensus:
  is_primary: false          # ✅ Change based on node role
  total_nodes: 7             # ✅ MUST match actual node count
  view_timeout: 5s           # ✅ Increase for high-latency networks
  checkpoint_interval: 100   # ✅ Adjust for storage efficiency
```

**Gossip Tuning:**
```yaml
gossip:
  fanout: 3                  # ✅ Increase for faster propagation
  ttl: 10                    # ✅ Increase for larger networks
  cache_size: 10000          # ✅ Increase if memory allows
  anti_entropy_interval: 5s  # ✅ Adjust for consistency needs
```

**Storage Configuration:**
```yaml
storage:
  path: "./data/db"          # ✅ Change to preferred location
  timeout: 10s               # ✅ Increase for slow disks
```

**Logging:**
```yaml
logging:
  level: "info"              # ✅ Change: debug, info, warn, error
  format: "json"             # ✅ Change: json or text
  output: "stdout"           # ✅ Change to file path
```

#### 2. Command-Line Flags

All flags are safe to change to match your deployment:
```bash
./bin/validator \
  -id mynode \               # ✅ Change
  -port 5001 \               # ✅ Change
  -api-port 9080 \           # ✅ Change
  -data-dir /var/lib/dtvn \  # ✅ Change
  -primary \                 # ✅ Add/remove based on role
  -total-nodes 10            # ✅ MUST match network size
```

#### 3. Docker Compose (`docker-compose.yml`)

**Safe Changes:**
- Port mappings (external ports)
- Volume paths
- Container names
- Environment variables
- Resource limits (add `resources:` section)

**Example:**
```yaml
validator-0:
  ports:
    - "5001:4001"            # ✅ Change external port
  volumes:
    - /opt/dtvn:/root/data   # ✅ Change host path
  container_name: my-validator-0  # ✅ Change name
  deploy:                    # ✅ Add resource limits
    resources:
      limits:
        cpus: '2'
        memory: 2G
```

#### 4. Monitoring (`config/prometheus.yml`)

```yaml
global:
  scrape_interval: 15s       # ✅ Change scraping frequency
  evaluation_interval: 15s   # ✅ Change evaluation frequency

scrape_configs:
  - job_name: 'validator-nodes'
    static_configs:
      - targets:             # ✅ Add/remove targets
          - 'validator-0:9090'
          - 'new-validator:9090'
```

### ⚠️ MODIFY WITH CAUTION (Implementation)

#### 1. State Machine (`pkg/state/machine.go`)

**Caution Areas:**
- State transition logic (lines 45-120)
- Vector clock updates (lines 125-180)
- Ticket validation rules (lines 60-85)

**Why:** Changes can break consensus or create inconsistencies

**If You Must Modify:**
- Add new states carefully
- Preserve existing transitions
- Update all validators simultaneously
- Test thoroughly with simulator

#### 2. PBFT Implementation (`pkg/consensus/pbft.go`)

**Caution Areas:**
- Three-phase protocol logic (lines 100-300)
- Quorum calculations (lines 50-80)
- View change mechanism (lines 350-450)

**Why:** PBFT correctness is critical for Byzantine tolerance

**If You Must Modify:**
- Understand PBFT paper thoroughly
- Maintain safety and liveness properties
- Test with Byzantine simulator

#### 3. Gossip Protocol (`pkg/gossip/engine.go`)

**Caution Areas:**
- Message propagation logic (lines 80-150)
- Cache management (lines 200-250)
- Anti-entropy reconciliation (lines 300-350)

**Why:** Affects message delivery guarantees

**Safe Modifications:**
- Fanout calculation (line 45) - adjust √n coefficient
- Cache size limits (line 30)
- TTL values (line 25)

#### 4. Network Layer (`pkg/network/host.go`)

**Caution Areas:**
- Protocol handler (lines 150-200)
- Stream management (lines 250-300)
- Security protocols (lines 100-130)

**Why:** Network security and reliability

**Safe Modifications:**
- Transport options (lines 50-80)
- Timeout values (lines 35-45)

### ❌ DO NOT CHANGE (Core Protocol)

#### 1. Protocol Buffers (`proto/messages.proto`)

**DO NOT modify existing message definitions:**
```protobuf
message PrePrepareMsg {     # ❌ DO NOT change
  uint64 view = 1;
  uint64 sequence = 2;
  bytes request_digest = 3;
  Request request = 4;
}
```

**Why:** Breaks compatibility between nodes

**If You Must Add Fields:**
- Only add NEW optional fields at the end
- Never remove or renumber existing fields
- Regenerate with `make proto`
- Update all nodes simultaneously

#### 2. Database Schema (`pkg/storage/store.go`)

**DO NOT change bucket names:**
```go
const (
    ticketsBucket     = []byte("tickets")      // ❌ DO NOT change
    consensusBucket   = []byte("consensus")    // ❌ DO NOT change
    checkpointsBucket = []byte("checkpoints")  // ❌ DO NOT change
    metadataBucket    = []byte("metadata")     // ❌ DO NOT change
)
```

**Why:** Breaks existing data

**For Schema Changes:**
- Implement migration logic
- Support backward compatibility
- Test with existing data

#### 3. Cryptographic Functions (`internal/crypto/signature.go`)

**DO NOT modify:**
- Key generation algorithm (Ed25519)
- Signature format
- Hash functions

**Why:** Breaks security and verification

#### 4. Peer Discovery Protocol (`pkg/network/discovery.go`)

**DO NOT change:**
- DHT protocol version
- Rendezvous point naming
- Advertisement format

**Why:** Breaks peer discovery

### Migration Guide for Breaking Changes

If you must make breaking changes:

**Step 1: Version the change**
```go
const ProtocolVersion = 2  // Increment
```

**Step 2: Support both versions**
```go
func HandleMessage(msg Message) {
    switch msg.Version {
    case 1:
        return handleV1(msg)
    case 2:
        return handleV2(msg)
    }
}
```

**Step 3: Coordinate rollout**
1. Deploy v2 nodes with v1 compatibility
2. Wait for all nodes to upgrade
3. Switch to v2-only mode
4. Remove v1 compatibility code

### Configuration Best Practices

**For Production:**
```yaml
consensus:
  view_timeout: 10s          # Increase for WAN deployments
  checkpoint_interval: 1000  # More frequent checkpoints

gossip:
  fanout: 5                  # Higher for reliability
  cache_size: 100000         # Larger cache

logging:
  level: "warn"              # Reduce verbosity
  format: "json"             # Structured logs
  output: "/var/log/dtvn/validator.log"

metrics:
  enabled: true              # Always enable in production
```

**For Development:**
```yaml
consensus:
  view_timeout: 5s           # Faster testing

gossip:
  fanout: 3                  # Smaller network

logging:
  level: "debug"             # Verbose debugging
  format: "text"             # Human-readable
  output: "stdout"

metrics:
  enabled: true              # Monitor during development
```

---

## Summary

### Key Takeaways

1. **DTVN is a Byzantine fault-tolerant ticket validation system** using PBFT consensus
2. **Requires Go 1.25.5+** to build and run
3. **Multiple deployment options:** Single node, multi-node, Docker, Docker Compose
4. **Tolerates f < n/3 Byzantine nodes** (e.g., 2 faults with 7 nodes)
5. **REST API** for ticket operations and WebSocket for real-time updates
6. **Built-in monitoring** with Prometheus and Grafana
7. **Comprehensive simulator** for testing Byzantine scenarios

### Quick Commands Reference

```bash
# Build
make build

# Run single node
make run-validator

# Run network of 4 nodes
make run-network

# Run simulator
make run-simulator

# Docker Compose (7 nodes + monitoring)
make docker-compose-up

# Tests
make test

# Clean up
make clean
```

### Getting Help

- **Code Issues:** File an issue on GitHub
- **Configuration Questions:** Check `config/validator.yaml` examples
- **API Questions:** See [API Reference](#api-reference) section
- **Performance Tuning:** See [What to Change](#what-to-change-vs-what-to-keep) section

### Next Steps

1. Read the existing README.md for architecture details
2. Review QUICKSTART.md for rapid deployment
3. Check IMPLEMENTATION_SUMMARY.md for development status
4. Run the simulator to understand Byzantine fault tolerance
5. Start with a single node, then scale to multi-node
6. Set up monitoring before production deployment

---

**Document Version:** 1.0
**Last Updated:** 2025-12-01
**DTVN Version:** Based on git commit 9e65833
