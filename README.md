# DTVN - Distributed Ticket Validation Network

A Byzantine Fault-Tolerant, peer-to-peer ticket validation system built with Go. DTVN uses PBFT consensus, gossip-based message dissemination, and libp2p networking to validate and track event tickets across a decentralized network of validator nodes — with no centralized infrastructure.

The system tolerates up to `f` Byzantine (malicious) nodes where `f = (n-1)/3` and `n` is the total number of validators.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Supervisor Dashboard](#supervisor-dashboard)
- [Running a Validator Network Manually](#running-a-validator-network-manually)
- [Network Simulator](#network-simulator)
- [API Reference](#api-reference)
- [WebSocket Events](#websocket-events)
- [Configuration](#configuration)
- [Byzantine Fault Tolerance](#byzantine-fault-tolerance)
- [Monitoring](#monitoring)
- [Testing](#testing)
- [CI/CD](#cicd)
- [Project Structure](#project-structure)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)
- [License](#license)
- [Contributing](#contributing)
- [References](#references)

## Features

- **Fully decentralized** — no central server, all nodes are equal peers
- **PBFT consensus** — three-phase commit (Pre-Prepare, Prepare, Commit) for ticket state agreement
- **Gossip protocol** — epidemic broadcast with anti-entropy for message dissemination and state reconciliation
- **libp2p networking** — TCP transport, Kademlia DHT peer discovery, secure channels, NAT traversal
- **Vector clocks** — causality tracking and conflict resolution for concurrent ticket updates
- **BoltDB persistence** — embedded key-value storage with atomic transactions
- **Web dashboard** — supervisor UI for managing nodes, viewing network topology, and monitoring metrics
- **Network simulator** — test Byzantine behavior, network partitions, and consensus performance
- **Prometheus metrics** — consensus latency, gossip throughput, peer counts, ticket operations
- **REST API + WebSocket** — per-node HTTP API and real-time event streaming

## Architecture

DTVN is organized into layers, each building on the one below:

```
Layer 6: API           REST endpoints + WebSocket (per-node)
Layer 5: Storage       BoltDB persistence (tickets, consensus log, checkpoints)
Layer 4: State         Ticket state machine + vector clocks
Layer 3: Consensus     PBFT (3-phase commit, view changes, checkpointing)
Layer 2: Gossip        Epidemic broadcast, anti-entropy, Bloom filter deduplication
Layer 1: Discovery     Kademlia DHT, bootstrap nodes
Layer 0: P2P           libp2p (TCP, TLS/Noise, connection multiplexing)
```

### Ticket Lifecycle

Tickets follow a strict state machine:

```bash
ISSUED ──→ VALIDATED ──→ CONSUMED
                │
                └──→ DISPUTED
```

- **ISSUED** — ticket created/seeded, awaiting validation
- **VALIDATED** — ticket passed PBFT consensus across the network
- **CONSUMED** — ticket used at the event gate
- **DISPUTED** — ticket flagged for review

State transitions are enforced by the state machine. Validation requires PBFT consensus (2f+1 agreement).

### Message Flow

1. Client sends `POST /api/v1/tickets/validate` to any node
2. Non-primary nodes forward the request via gossip to all peers
3. The primary node initiates PBFT consensus:
   - **Pre-Prepare** → **Prepare** (wait for 2f+1) → **Commit** (wait for 2f+1) → **Execute**
4. After 2f+1 commits, each node applies the state change locally
5. Anti-entropy (every 5s) ensures eventual consistency across all nodes

## Prerequisites

- **Go 1.25+**
- **Make** (for build automation)
- **protoc** (optional, only needed if modifying protocol buffer definitions)

## Installation

```bash
# Clone the repository
git clone https://github.com/Hetti219/DTVN.git
cd DTVN

# Install dependencies
go mod download

# Build all binaries (validator, simulator, supervisor)
make build
```

This produces three binaries in `bin/`:

- `bin/validator` — a single validator node
- `bin/simulator` — network simulation tool
- `bin/supervisor` — web dashboard for managing a cluster

## Quick Start

The fastest way to get started is with the supervisor dashboard:

```bash
make build
./bin/supervisor
```

Open http://localhost:8080 in your browser. From the dashboard you can:

- Start/stop a cluster of validator nodes
- Seed test tickets and validate/consume/dispute them
- View the network topology graph
- Run the Byzantine fault tolerance simulator
- Monitor real-time metrics and node logs

## Supervisor Dashboard

The supervisor is a separate process that manages validator node subprocesses and provides a web UI.

### CLI Flags

| Flag          | Default       | Description                                  |
| ------------- | ------------- | -------------------------------------------- |
| `-web-port`   | `8080`        | Web interface port                           |
| `-web-addr`   | `0.0.0.0`     | Web interface bind address                   |
| `-data-dir`   | `./data`      | Data directory for node databases            |
| `-validator`  | auto-detected | Path to validator binary                     |
| `-simulator`  | auto-detected | Path to simulator binary                     |
| `-static-dir` | auto-detected | Path to static web files                     |
| `-auto-start` | `0`           | Auto-start N nodes on startup (0 = disabled) |

### Dashboard Pages

- **Dashboard** — cluster overview, quick actions (start cluster, seed tickets, validate)
- **Nodes** — start/stop/restart individual nodes, view logs
- **Tickets** — browse all tickets, validate/consume/dispute actions
- **Network** — D3.js force-directed graph of the P2P mesh topology
- **Metrics** — Chart.js graphs of consensus latency, ticket throughput, gossip stats
- **Simulator** — configure and run Byzantine fault tolerance simulations

## Running a Validator Network Manually

### Single Node

```bash
./bin/validator \
  -id node0 \
  -port 4001 \
  -api-port 8081 \
  -data-dir ./data/node0 \
  -primary \
  -total-nodes 4
```

### Multi-Node Network

Peer IDs are deterministic based on node ID. `node0` always has peer ID `12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C`.

```bash
# Terminal 1: Primary node
./bin/validator -id node0 -port 4001 -api-port 8081 -data-dir ./data/node0 \
  -primary -total-nodes 4

# Terminal 2: Validator node 1
./bin/validator -id node1 -port 4002 -api-port 8082 -data-dir ./data/node1 \
  -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"

# Terminal 3: Validator node 2
./bin/validator -id node2 -port 4003 -api-port 8083 -data-dir ./data/node2 \
  -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"

# Terminal 4: Validator node 3
./bin/validator -id node3 -port 4004 -api-port 8084 -data-dir ./data/node3 \
  -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

Or use the Makefile shortcut:

```bash
make run-network    # Starts a 4-node network (node0-node3)
```

### Validator CLI Flags

| Flag              | Default  | Description                                    |
| ----------------- | -------- | ---------------------------------------------- |
| `-id`             | `node0`  | Node identifier                                |
| `-port`           | `4001`   | P2P listen port                                |
| `-api-port`       | `8080`   | REST API server port                           |
| `-data-dir`       | `./data` | Data directory for BoltDB storage              |
| `-bootstrap`      | —        | Comma-separated bootstrap peer multiaddrs      |
| `-bootstrap-node` | `false`  | Run as a bootstrap-only node                   |
| `-primary`        | `false`  | Run as the primary (leader) node               |
| `-total-nodes`    | `4`      | Total number of validator nodes in the network |

## Network Simulator

The simulator tests Byzantine fault tolerance, network partitions, and consensus performance in a controlled environment.

```bash
# Basic simulation: 7 nodes, 2 Byzantine, 100 tickets
./bin/simulator \
  -nodes 7 \
  -byzantine 2 \
  -tickets 100 \
  -duration 60s \
  -latency 50ms \
  -packet-loss 0.01

# With network partitions
./bin/simulator \
  -nodes 7 \
  -byzantine 2 \
  -tickets 100 \
  -duration 120s \
  -latency 100ms \
  -packet-loss 0.05 \
  -partition
```

Or via the Makefile:

```bash
make run-simulator            # Standard simulation
make run-simulator-partition  # Simulation with network partitions
```

### Simulator Flags

| Flag           | Default | Description                                           |
| -------------- | ------- | ----------------------------------------------------- |
| `-nodes`       | `7`     | Number of validator nodes                             |
| `-byzantine`   | `2`     | Number of Byzantine (malicious) nodes (must be < n/3) |
| `-tickets`     | `100`   | Number of tickets to simulate                         |
| `-duration`    | `60s`   | Simulation duration                                   |
| `-latency`     | `50ms`  | Simulated network latency                             |
| `-packet-loss` | `0.01`  | Packet loss rate (0.0–1.0)                            |
| `-partition`   | `false` | Enable network partition simulation                   |

## API Reference

Each validator node exposes a REST API. When using the supervisor, these endpoints are proxied through the supervisor's single port.

### Ticket Operations

**Seed test tickets**

```bash
curl -X POST http://localhost:8081/api/v1/tickets/seed
```

Seeds 500 deterministic tickets in ISSUED state.

**Validate a ticket**

```bash
curl -X POST http://localhost:8081/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001", "data": "eyJldmVudCI6IkNvbmNlcnQifQ=="}'
```

**Consume a ticket**

```bash
curl -X POST http://localhost:8081/api/v1/tickets/consume \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001"}'
```

**Dispute a ticket**

```bash
curl -X POST http://localhost:8081/api/v1/tickets/dispute \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001"}'
```

**Get a ticket**

```bash
curl http://localhost:8081/api/v1/tickets/TICKET-001
```

**Get all tickets**

```bash
curl http://localhost:8081/api/v1/tickets
```

### Node Info

**Node statistics**

```bash
curl http://localhost:8081/api/v1/stats
```

Returns: `node_id`, `peer_count`, `is_primary`, `current_view`, `sequence`, `cache_size`, `storage_stats`.

**Connected peers**

```bash
curl http://localhost:8081/api/v1/peers
```

**Node configuration**

```bash
curl http://localhost:8081/api/v1/config
```

**API status**

```bash
curl http://localhost:8081/api/v1/status
```

**Health check**

```bash
curl http://localhost:8081/health
```

### Supervisor-Only Endpoints

When using the supervisor dashboard, additional management endpoints are available:

| Method   | Endpoint                         | Description             |
| -------- | -------------------------------- | ----------------------- |
| `GET`    | `/api/v1/nodes`                  | List all nodes          |
| `POST`   | `/api/v1/nodes`                  | Start a new node        |
| `GET`    | `/api/v1/nodes/{nodeID}`         | Get node details        |
| `DELETE` | `/api/v1/nodes/{nodeID}`         | Stop a node             |
| `POST`   | `/api/v1/nodes/{nodeID}/restart` | Restart a node          |
| `GET`    | `/api/v1/nodes/{nodeID}/logs`    | Get node logs           |
| `POST`   | `/api/v1/cluster/start`          | Start a cluster (async) |
| `POST`   | `/api/v1/cluster/stop`           | Stop all nodes          |
| `GET`    | `/api/v1/simulator`              | Get simulator status    |
| `POST`   | `/api/v1/simulator/start`        | Start simulator         |
| `POST`   | `/api/v1/simulator/stop`         | Stop simulator          |
| `GET`    | `/api/v1/simulator/output`       | Get simulator output    |
| `GET`    | `/api/v1/simulator/results`      | Get simulator results   |

## WebSocket Events

Connect to `ws://localhost:8081/ws` (validator) or `ws://localhost:8080/ws` (supervisor) for real-time updates.

```javascript
const ws = new WebSocket("ws://localhost:8080/ws");

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log(msg.type, msg);
};
```

### Validator Events

| Event              | Description                          |
| ------------------ | ------------------------------------ |
| `ticket_validated` | A ticket was validated via consensus |
| `ticket_consumed`  | A ticket was consumed                |
| `ticket_disputed`  | A ticket was disputed                |
| `tickets_seeded`   | Test tickets were seeded             |

### Supervisor Events

| Event                | Description                                                     |
| -------------------- | --------------------------------------------------------------- |
| `connected`          | Initial connection (includes current node list, cluster status) |
| `node_status`        | Node started/stopped/errored                                    |
| `node_output`        | Console output from a node process                              |
| `cluster_status`     | Cluster startup progress (`nodes_started` / `nodes_total`)      |
| `simulator_status`   | Simulator started/stopped                                       |
| `simulator_output`   | Console output from the simulator                               |
| `simulator_progress` | Simulation progress update                                      |
| `simulator_results`  | Final simulation results                                        |
| `ticket_validated`   | Proxied ticket validation event                                 |
| `ticket_consumed`    | Proxied ticket consumption event                                |
| `ticket_disputed`    | Proxied ticket dispute event                                    |
| `tickets_seeded`     | Proxied ticket seeding event                                    |
| `pong`               | Keep-alive response                                             |

## Configuration

### Validator Config File

Edit `config/validator.yaml`:

```yaml
node:
  id: "validator-1"
  data_dir: "./data/validator-1"

network:
  listen_port: 4001
  bootstrap_mode: false
  bootstrap_peers:
    - "/ip4/127.0.0.1/tcp/4000/p2p/..."

api:
  address: "0.0.0.0"
  port: 8080
  enable_cors: true

consensus:
  is_primary: false
  total_nodes: 7
  view_timeout: 5s
  checkpoint_interval: 100

gossip:
  fanout: 3
  ttl: 10
  cache_size: 10000
  anti_entropy_interval: 5s

storage:
  type: "boltdb"
  path: "./data/validator-1/db"
  timeout: 10s

metrics:
  enabled: true
  address: "0.0.0.0"
  port: 9090

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

### Prometheus Config

A sample Prometheus scrape configuration is provided in `config/prometheus.yml` for monitoring validator nodes.

## Byzantine Fault Tolerance

The system uses PBFT to tolerate Byzantine (arbitrarily malicious) nodes. The tolerance formula is:

```go
f = (n - 1) / 3
```

where `f` is the maximum number of Byzantine nodes tolerated and `n` is the total node count.

| Nodes (n) | Byzantine Tolerance (f) | Quorum Required (2f+1) |
| --------- | ----------------------- | ---------------------- |
| 4         | 1                       | 3                      |
| 7         | 2                       | 5                      |
| 10        | 3                       | 7                      |
| 13        | 4                       | 9                      |

**Minimum network size:** 4 nodes (tolerates 1 Byzantine node).

### PBFT Protocol

- **Primary election:** `primary = view % totalNodes`
- **View changes:** triggered on 15s timeout if consensus stalls
- **Checkpointing:** every 10 consensus operations for garbage collection
- **Sequence numbers:** monotonically increasing; gaps block consensus until recovered via anti-entropy

## Monitoring

### Prometheus Metrics

Each validator exposes metrics at its configured metrics port (default `9090`):

- **Consensus** — rounds total, success/failure counts, latency histogram
- **Gossip** — messages sent/received, cache size, hits/misses
- **Network** — peer count, message count, bytes transmitted
- **Tickets** — validated/consumed/disputed counters, state gauges
- **Storage** — operation counts, latency histograms
- **API** — request counts, latency histograms, WebSocket connection count

## Testing

```bash
# Run all tests
make test

# Unit tests only (fast)
make test-unit

# Tests with HTML coverage report (opens coverage.html)
make test-coverage

# Integration tests
make test-integration

# Run a specific test
go test -v -run TestSpecificFunction ./pkg/consensus/

# Run a specific integration test
make test-integration-single TEST=TestSingleTicketValidation
```

### Integration Test Options

The integration test runner (`scripts/run-integration-tests.sh`) supports:

| Flag                 | Description                       |
| -------------------- | --------------------------------- |
| `-p`                 | Run tests in parallel             |
| `-v`                 | Verbose output                    |
| `--no-cleanup`       | Keep test artifacts for debugging |
| `--pattern PATTERN`  | Run only matching test names      |
| `--timeout DURATION` | Override default 30m timeout      |

## CI/CD

### Continuous Integration

Every push and pull request to `main` triggers the CI pipeline (`.github/workflows/go-ci.yml`):

1. Checkout code
2. Set up Go 1.25.5
3. Download dependencies
4. Build all binaries (`make build`)
5. Run all tests (`make test`)

### Releases

Pushing a version tag (`v*`) triggers the release pipeline (`.github/workflows/release.yml`):

- Builds `dtvn-validator` and `dtvn-simulator` for Linux amd64/arm64
- Signs artifacts with Cosign
- Generates SBOMs with Syft
- Creates a GitHub Release with changelog (grouped by category)

## Project Structure

```bash
.
├── cmd/
│   ├── validator/          # Validator node entry point
│   ├── simulator/          # Network simulator entry point
│   └── supervisor/         # Web dashboard entry point
├── pkg/
│   ├── api/                # REST API + WebSocket (per-node)
│   ├── consensus/          # PBFT consensus implementation
│   ├── gossip/             # Gossip protocol (epidemic broadcast, anti-entropy)
│   ├── network/            # libp2p host, DHT discovery, connection management
│   ├── state/              # Ticket state machine, vector clocks
│   ├── storage/            # BoltDB persistence layer
│   └── supervisor/         # Supervisor server, node manager, API proxy
├── internal/
│   ├── crypto/             # Ed25519 key generation, signing, verification
│   ├── types/              # Shared types (NodeInfo, TicketMetadata, errors)
│   └── metrics/            # Prometheus metric definitions and server
├── web/
│   └── static/             # Frontend SPA (HTML, CSS, JS)
│       ├── index.html
│       ├── css/            # Stylesheets
│       └── js/             # Modules (app, api, websocket, dashboard, etc.)
├── proto/                  # Protocol buffer definitions
├── config/                 # Configuration templates (validator.yaml, prometheus.yml)
├── scripts/                # Build and test scripts
├── test/                   # Integration test scenarios
├── .github/                # CI/CD workflows, PR template, dependabot
├── Makefile                # Build automation
├── .goreleaser.yaml        # Release configuration
└── LICENSE                 # MIT License
```

## Performance

### Benchmarks (7-node network)

| Metric            | Value                       |
| ----------------- | --------------------------- |
| Consensus latency | ~150ms (3 PBFT round-trips) |
| Throughput        | ~100 tickets/second         |
| Gossip fanout     | sqrt(n) peers per round     |
| Message overhead  | O(n^2) per consensus round  |

### Scalability

| Operation                 | Complexity      |
| ------------------------- | --------------- |
| Peer discovery (Kademlia) | O(log n)        |
| Gossip dissemination      | O(n log n)      |
| PBFT consensus            | O(n^2) messages |
| Storage                   | O(tickets)      |

## Troubleshooting

### Peers not connecting

- Verify bootstrap peer addresses and peer IDs
- Check that P2P ports are not blocked by a firewall
- Ensure ports are not already in use by another process

### Consensus not reaching agreement

- Network must have at least 4 nodes
- Byzantine node count must be strictly less than n/3
- Check that the primary node is reachable
- Review node logs for PBFT timeout or view change messages

### High latency

- Reduce the simulated network latency parameter
- Increase gossip fanout for faster dissemination
- Check system resources (CPU, memory, disk I/O)

### Stale state across nodes

- Anti-entropy runs every 5 seconds — wait for sync
- Use `curl http://localhost:8081/api/v1/stats` to check peer count and sequence numbers
- Restart lagging nodes if sequence gaps are large

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes and add tests
4. Run `make fmt && make lint && make test`
5. Commit your changes
6. Push to your branch and open a Pull Request

## References

- [Practical Byzantine Fault Tolerance (Castro & Liskov, 1999)](http://pmg.csail.mit.edu/papers/osdi99.pdf)
- [libp2p Specifications](https://github.com/libp2p/specs)
- [Epidemic Algorithms for Replicated Database Maintenance (Demers et al., 1987)](https://dl.acm.org/doi/10.1145/41840.41841)
- [Vector Clocks](https://en.wikipedia.org/wiki/Vector_clock)
- [Kademlia: A Peer-to-peer Information System Based on the XOR Metric](https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf)
