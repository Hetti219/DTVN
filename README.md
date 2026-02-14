# DTVN - Distributed Ticket Validation Network

A Byzantine Fault-Tolerant, peer-to-peer ticket validation system built with Go. DTVN uses PBFT consensus, gossip-based message dissemination, and libp2p networking to validate and track event tickets across a decentralized network of validator nodes -- with no centralized infrastructure.

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
- [Testing](#testing)
- [CI/CD](#cicd)
- [Project Structure](#project-structure)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## Features

- **Fully decentralized** -- no central server, all nodes are equal peers
- **PBFT consensus** -- three-phase commit (Pre-Prepare, Prepare, Commit) for ticket state agreement
- **Gossip protocol** -- epidemic broadcast with anti-entropy for message dissemination and state reconciliation
- **libp2p networking** -- TCP transport, Kademlia DHT peer discovery, Noise/TLS encryption, NAT traversal
- **Vector clocks** -- causality tracking and conflict resolution for concurrent ticket updates
- **BoltDB persistence** -- embedded key-value storage with atomic transactions; state survives node restarts
- **State replication** -- reliable post-consensus broadcast to all peers, plus periodic anti-entropy sync
- **Web dashboard** -- supervisor UI for managing nodes, viewing network topology, and monitoring metrics
- **Network simulator** -- test Byzantine behavior, network partitions, and consensus performance
- **REST API + WebSocket** -- per-node HTTP API with real-time event streaming
- **API key authentication** -- optional constant-time-comparison API key auth via header or Bearer token
- **WebSocket origin checking** -- prevents cross-site WebSocket hijacking; configurable allowed origins
- **Deterministic peer IDs** -- same node ID always produces the same cryptographic identity for stable bootstrap addresses

## Architecture

DTVN is organized into layers, each building on the one below:

```
Layer 7: Supervisor      Web dashboard, node lifecycle management, API proxy
Layer 6: API             REST endpoints + WebSocket (per-node)
Layer 5: Validator Node  The glue -- wires all layers, implements ValidatorInterface
Layer 4: State Machine   Ticket lifecycle + vector clocks for causality
Layer 3: Consensus       PBFT (3-phase commit, view changes, callbacks)
Layer 2: Gossip          Epidemic broadcast, anti-entropy, Bloom filter deduplication
Layer 1: P2P Network     libp2p host, Kademlia DHT discovery, message routing
Layer 0: Storage         BoltDB persistence (tickets, consensus log, checkpoints)
```

### Ticket Lifecycle

Tickets follow a strict state machine:

```bash
ISSUED ----> VALIDATED ----> CONSUMED
                  |
                  +----> DISPUTED
```

| State         | Meaning                                                |
| ------------- | ------------------------------------------------------ |
| **ISSUED**    | Ticket sold/purchased, awaiting validation at the gate |
| **VALIDATED** | Ticket passed PBFT consensus across the network        |
| **CONSUMED**  | Ticket used at the event gate                          |
| **DISPUTED**  | Ticket flagged for review (possible fraud)             |

State transitions are enforced inline within the state machine. Validation requires PBFT consensus (2f+1 agreement).

### Message Flow

1. Client sends `POST /api/v1/tickets/validate` to any node
2. **Non-primary nodes** serialize the request as a `CLIENT_REQUEST` protobuf and broadcast to all peers
3. **The primary node** initiates PBFT consensus:
   - **Pre-Prepare** (primary broadcasts proposal)
   - **Prepare** (replicas validate and vote; wait for 2f+1)
   - **Commit** (nodes confirm; wait for 2f+1)
   - **Execute** (all nodes apply the state change locally)
4. **State Replicator** broadcasts committed state directly to all peers (reliable delivery)
5. **Anti-entropy** (every 5s) ensures eventual consistency for any missed updates
6. Non-primary nodes poll their local state until the validated ticket appears, then return success to the client

### Protobuf Message Types

All network communication uses Protocol Buffers. The `ValidatorMessage` wrapper carries one of 10 message types:

| Type                      | Purpose                      |
| ------------------------- | ---------------------------- |
| `PRE_PREPARE` (0)         | Primary's consensus proposal |
| `PREPARE` (1)             | Replica agreement vote       |
| `COMMIT` (2)              | Final commit vote            |
| `VIEW_CHANGE` (3)         | Leader election trigger      |
| `CHECKPOINT` (4)          | State snapshot               |
| `STATE_UPDATE` (5)        | Gossip state propagation     |
| `HEARTBEAT` (6)           | Keep-alive                   |
| `CLIENT_REQUEST` (7)      | Forwarded validation request |
| `STATE_SYNC_REQUEST` (8)  | Anti-entropy request         |
| `STATE_SYNC_RESPONSE` (9) | Anti-entropy response        |

## Prerequisites

- **Go 1.25+**
- **Make** (for build automation)
- **protoc** (optional, only needed if modifying `.proto` files)

## Installation

```bash
git clone https://github.com/Hetti219/DTVN.git
cd DTVN

go mod download

make build
```

This produces three binaries in `bin/`:

| Binary           | Description                          |
| ---------------- | ------------------------------------ |
| `bin/validator`  | A single validator node              |
| `bin/simulator`  | Network simulation tool              |
| `bin/supervisor` | Web dashboard for managing a cluster |

## Quick Start

The fastest way to get started is with the supervisor dashboard:

```bash
make build
./bin/supervisor
```

Open http://localhost:8080 in your browser. From the dashboard you can:

- Start/stop a cluster of validator nodes
- Seed 500 test tickets and validate/consume/dispute them
- View the P2P network topology graph (D3.js)
- Run the Byzantine fault tolerance simulator
- Monitor real-time metrics and node logs

## Supervisor Dashboard

The supervisor is a separate process that manages validator node subprocesses and provides a web UI.

### Supervisor CLI Flags

| Flag          | Default       | Description                                        |
| ------------- | ------------- | -------------------------------------------------- |
| `-web-port`   | `8080`        | Web interface port                                 |
| `-web-addr`   | `0.0.0.0`     | Web interface bind address                         |
| `-data-dir`   | `./data`      | Data directory for node databases                  |
| `-validator`  | auto-detected | Path to validator binary                           |
| `-simulator`  | auto-detected | Path to simulator binary                           |
| `-static-dir` | auto-detected | Path to static web files                           |
| `-auto-start` | `0`           | Auto-start N nodes on startup (0 = disabled)       |
| `-api-key`    | `""`          | API key for authentication (or set `DTVN_API_KEY`) |

### Dashboard Pages

| Page          | Description                                                             |
| ------------- | ----------------------------------------------------------------------- |
| **Dashboard** | Cluster overview, quick actions (start cluster, seed tickets, validate) |
| **Nodes**     | Start/stop/restart individual nodes, view live logs                     |
| **Tickets**   | Browse all tickets, validate/consume/dispute actions                    |
| **Network**   | D3.js force-directed graph of the P2P mesh topology                     |
| **Metrics**   | Chart.js graphs of consensus latency, ticket throughput, gossip stats   |
| **Simulator** | Configure and run Byzantine fault tolerance simulations                 |

### How the Supervisor Works

- **Cluster startup is asynchronous**: `StartCluster()` returns immediately, starts nodes in the background, and sends progress via WebSocket `cluster_status` events.
- **Ticket mutations route to the PRIMARY node only** via `getPrimaryNodeURL()`. Reads go to any running node. Seed operations go to ALL nodes.
- **WebSocket writes are serialized** with a `wsWriteMu` mutex (gorilla/websocket is not thread-safe for concurrent writes).

## Running a Validator Network Manually

### Single Node

```bash
./bin/validator \
  -id node0 \
  -port 4001 \
  -api-port 8081 \
  -data-dir ./data/node0 \
  -primary \
  -total-nodes 1
```

### Multi-Node Network (4 Nodes)

Peer IDs are deterministic based on node ID. `node0` always has peer ID `12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C`.

```bash
# Terminal 1: Primary node
./bin/validator -id node0 -port 4001 -api-port 8081 -data-dir ./data/node0 \
  -primary -total-nodes 4

# Terminal 2
./bin/validator -id node1 -port 4002 -api-port 8082 -data-dir ./data/node1 \
  -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"

# Terminal 3
./bin/validator -id node2 -port 4003 -api-port 8083 -data-dir ./data/node2 \
  -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"

# Terminal 4
./bin/validator -id node3 -port 4004 -api-port 8084 -data-dir ./data/node3 \
  -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

Or use the Makefile shortcut:

```bash
make run-network
```

### Validator CLI Flags

| Flag              | Default  | Description                                        |
| ----------------- | -------- | -------------------------------------------------- |
| `-id`             | `node0`  | Node identifier                                    |
| `-port`           | `4001`   | P2P listen port                                    |
| `-api-port`       | `8080`   | REST API server port                               |
| `-data-dir`       | `./data` | Data directory for BoltDB storage                  |
| `-bootstrap`      | --       | Comma-separated bootstrap peer multiaddrs          |
| `-bootstrap-node` | `false`  | Run as a bootstrap-only node                       |
| `-primary`        | `false`  | Run as the primary (leader) node                   |
| `-total-nodes`    | `4`      | Total number of validator nodes in the network     |
| `-api-key`        | `""`     | API key for authentication (or set `DTVN_API_KEY`) |

### Startup Behavior

On startup, each validator node:

1. Initializes BoltDB storage and loads persisted tickets into the state machine
2. Creates a libp2p host with a deterministic Ed25519 key derived from the node ID
3. Connects to bootstrap peers and starts Kademlia DHT discovery
4. Waits up to 15 seconds for expected peers to connect (logging progress)
5. Starts the gossip engine, PBFT consensus, and API server
6. Immediately requests state synchronization from a random peer
7. Begins periodic anti-entropy sync every 5 seconds

## Network Simulator

The simulator tests Byzantine fault tolerance, network partitions, and consensus performance.

```bash
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
make run-simulator               # Standard simulation
make run-simulator-partition     # With network partitions
```

### Simulator Flags

| Flag           | Default | Description                               |
| -------------- | ------- | ----------------------------------------- |
| `-nodes`       | `7`     | Number of validator nodes                 |
| `-byzantine`   | `2`     | Number of Byzantine nodes (must be < n/3) |
| `-tickets`     | `100`   | Number of tickets to simulate             |
| `-duration`    | `60s`   | Simulation duration                       |
| `-latency`     | `50ms`  | Simulated network latency                 |
| `-packet-loss` | `0.01`  | Packet loss rate (0.0-1.0)                |
| `-partition`   | `false` | Enable network partition simulation       |

The simulator validates that Byzantine nodes cannot exceed `(n-1)/3` and exits with an error if violated.

## API Reference

Each validator node exposes a REST API. When using the supervisor, these endpoints are proxied through the supervisor's port.

### Ticket Operations

| Method | Endpoint                   | Description                                 |
| ------ | -------------------------- | ------------------------------------------- |
| `POST` | `/api/v1/tickets/validate` | Validate a ticket (triggers PBFT consensus) |
| `POST` | `/api/v1/tickets/consume`  | Mark a validated ticket as consumed         |
| `POST` | `/api/v1/tickets/dispute`  | Dispute a ticket                            |
| `POST` | `/api/v1/tickets/seed`     | Seed 500 deterministic test tickets         |
| `GET`  | `/api/v1/tickets/{id}`     | Get a single ticket                         |
| `GET`  | `/api/v1/tickets`          | Get all tickets                             |

### Node Information

| Method | Endpoint                  | Description                                               |
| ------ | ------------------------- | --------------------------------------------------------- |
| `GET`  | `/api/v1/status`          | Node status (uptime, WS client count)                     |
| `GET`  | `/api/v1/stats`           | Node statistics (peer count, view, sequence, cache size)  |
| `GET`  | `/api/v1/peers`           | Connected peers (IDs and addresses)                       |
| `GET`  | `/api/v1/config`          | Node configuration (node ID, total nodes, primary status) |
| `GET`  | `/api/v1/consensus/logs`  | PBFT consensus history from BoltDB                        |
| `GET`  | `/api/v1/node/crypto`     | Cryptographic identity (peer ID, public key, algorithms)  |
| `GET`  | `/api/v1/storage/entries` | Storage statistics (ticket counts by state, DB stats)     |
| `GET`  | `/health`                 | Health check                                              |

### Supervisor-Only Endpoints

| Method   | Endpoint                         | Description             |
| -------- | -------------------------------- | ----------------------- |
| `GET`    | `/api/v1/nodes`                  | List all managed nodes  |
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

### Example Requests

```bash
# Seed 500 test tickets
curl -X POST http://localhost:8081/api/v1/tickets/seed

# Validate a ticket
curl -X POST http://localhost:8081/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001"}'

# Consume a ticket
curl -X POST http://localhost:8081/api/v1/tickets/consume \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001"}'

# Dispute a ticket
curl -X POST http://localhost:8081/api/v1/tickets/dispute \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001"}'

# Get a ticket
curl http://localhost:8081/api/v1/tickets/TICKET-001

# Check node stats
curl http://localhost:8081/api/v1/stats | jq .

# Check peer connections
curl http://localhost:8081/api/v1/stats | jq .peer_count
```

### Authentication

When an API key is configured (via `-api-key` flag or `DTVN_API_KEY` env var), all `/api/` routes require authentication. Pass the key via:

- `X-API-Key` header: `curl -H "X-API-Key: YOUR_KEY" ...`
- `Authorization` header: `curl -H "Authorization: Bearer YOUR_KEY" ...`

Exempt paths: `/health`, `/ws`, and static files.

## WebSocket Events

Connect to `ws://localhost:8081/ws` (validator) or `ws://localhost:8080/ws` (supervisor) for real-time updates.

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

### WebSocket Origin Policy

The server validates WebSocket origins to prevent cross-site hijacking:

- Same-origin and localhost connections are always allowed
- Additional origins can be allowed via the `DTVN_WS_ORIGINS` environment variable (comma-separated)
- Non-browser clients (no `Origin` header) are always allowed

## Configuration

### Environment Variables

| Variable          | Description                                                              |
| ----------------- | ------------------------------------------------------------------------ |
| `DTVN_API_KEY`    | API key for authentication (alternative to `-api-key` flag)              |
| `DTVN_KEY_SEED`   | Secret salt for deterministic key generation (default: development salt) |
| `DTVN_WS_ORIGINS` | Comma-separated list of additional allowed WebSocket origins             |

### Config Files

- `config/validator.yaml` -- Validator node configuration template
- `config/prometheus.yml` -- Prometheus scrape configuration for monitoring

## Byzantine Fault Tolerance

The system uses PBFT to tolerate Byzantine (arbitrarily malicious) nodes:

```
f = (n - 1) / 3          Maximum Byzantine nodes tolerated
n = 3f + 1               Minimum nodes needed for f faults
quorum = 2f + 1          Votes required for agreement
```

| Nodes (n) | Byzantine Tolerance (f) | Quorum Required (2f+1) |
| --------- | ----------------------- | ---------------------- |
| 4         | 1                       | 3                      |
| 7         | 2                       | 5                      |
| 10        | 3                       | 7                      |
| 13        | 4                       | 9                      |

### PBFT Details

- **Primary election**: `primary = view % totalNodes` (deterministic, all nodes compute the same result)
- **View changes**: Triggered on 15-second timeout if consensus stalls while in a non-idle state
- **Quorum checks**: Primary verifies `activeNodes >= 2f+1` before proposing
- **Message reordering**: PREPAREs and COMMITs that arrive before PRE-PREPARE are stored and processed when the PRE-PREPARE arrives
- **Synchronous API responses**: Primary registers a callback per request and waits up to 20 seconds for consensus to complete
- **State replication**: After consensus commits, the StateReplicator broadcasts the committed state directly to all peers (guaranteed delivery, not probabilistic gossip)

## Testing

```bash
make test                   # All tests (unit + integration)
make test-unit              # Unit tests only (fast): go test -v -short ./...
make test-coverage          # Tests with HTML coverage report
make test-integration       # Integration tests via scripts/run-integration-tests.sh

# Run a specific test
go test -v -run TestSpecificFunction ./pkg/consensus/

# Run a specific integration test
make test-integration-single TEST=TestSingleTicketValidation
```

### Test Suite

**Unit tests** (9 files):

- `pkg/consensus/pbft_test.go` -- PBFT 3-phase commit, quorum, view changes
- `pkg/gossip/cache_test.go` -- LRU cache and Bloom filter
- `pkg/gossip/engine_test.go` -- Gossip dissemination and anti-entropy
- `pkg/network/host_test.go` -- P2P host, message sending, peer management
- `pkg/network/router_test.go` -- Message routing by type
- `pkg/state/machine_test.go` -- State transitions, conflict resolution
- `pkg/state/vectorclock_test.go` -- Vector clock ordering and merge
- `pkg/storage/store_test.go` -- BoltDB CRUD operations
- `pkg/api/server_test.go` -- API handlers and middleware

**Integration tests** (7 scenarios in `test/integration/scenarios/`):

- Single ticket validation
- Double validation prevention
- Concurrent validation
- State synchronization across nodes
- Peer discovery
- Byzantine fault tolerance
- Network partition recovery

### Integration Test Options

| Flag                 | Description                       |
| -------------------- | --------------------------------- |
| `-p`                 | Run tests in parallel             |
| `-v`                 | Verbose output                    |
| `--no-cleanup`       | Keep test artifacts for debugging |
| `--pattern PATTERN`  | Run only matching test names      |
| `--timeout DURATION` | Override default timeout          |

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

- Builds binaries via GoReleaser for multiple platforms
- Signs artifacts with Cosign
- Generates SBOMs with Syft
- Creates a GitHub Release with auto-generated changelog via git-cliff

## Project Structure

```bash
.
├── cmd/
│   ├── validator/            # Validator node entry point (main.go -- 1313 lines)
│   ├── simulator/            # Network simulator entry point
│   └── supervisor/           # Web dashboard entry point
├── pkg/
│   ├── api/                  # REST API + WebSocket (server.go, peers.go, static.go)
│   ├── consensus/            # PBFT implementation (pbft.go, messages.go, quorum.go)
│   ├── gossip/               # Gossip protocol (engine.go, cache.go)
│   ├── network/              # libp2p host, DHT discovery, message routing
│   ├── state/                # Ticket state machine + vector clocks
│   ├── storage/              # BoltDB persistence layer
│   └── supervisor/           # Supervisor server, node manager, API proxy, simulator control
├── internal/
│   ├── crypto/               # Ed25519 key generation, signing, verification
│   ├── types/                # Shared types (NodeInfo, error types, metrics structs)
│   └── metrics/              # Prometheus metric definitions
├── proto/
│   ├── messages.proto        # Protocol Buffer definitions (17 message types)
│   └── messages.pb.go        # Generated Go code
├── web/
│   └── static/               # Frontend SPA
│       ├── index.html
│       ├── css/              # Stylesheets (main, components, dashboard)
│       └── js/               # Modules (app, api, websocket, dashboard, metrics,
│                             #          network-viz, nodes, tickets, simulator)
├── test/
│   └── integration/          # Integration test scenarios, runner, fixtures
├── config/                   # Configuration templates (validator.yaml, prometheus.yml)
├── scripts/                  # Build and test scripts
├── docs/                     # Detailed code explanations by layer
├── .github/                  # CI/CD workflows, PR template, dependabot
├── Makefile                  # Build automation (make help for all targets)
├── .goreleaser.yaml          # Release configuration
└── LICENSE                   # MIT License
```

### Codebase Stats

| Metric              | Count            |
| ------------------- | ---------------- |
| Go source files     | 46               |
| Go source lines     | ~11,600          |
| Go test lines       | ~9,500           |
| Functions + methods | 713              |
| Structs             | 103              |
| Interfaces          | 4                |
| JS files            | 9 (~2,950 lines) |
| Protobuf messages   | 17 types         |

## Performance

### Benchmarks (7-node network)

| Metric            | Value                       |
| ----------------- | --------------------------- |
| Consensus latency | ~150ms (3 PBFT round-trips) |
| Throughput        | ~100 tickets/second         |
| Gossip fanout     | sqrt(n) peers per round     |
| Message overhead  | O(n^2) per consensus round  |

### Scalability

| Operation                     | Complexity      |
| ----------------------------- | --------------- |
| Peer discovery (Kademlia DHT) | O(log n)        |
| Gossip dissemination          | O(n log n)      |
| PBFT consensus                | O(n^2) messages |
| Storage (BoltDB)              | O(tickets)      |

## Troubleshooting

### Peers not connecting

- Verify bootstrap addresses include the peer ID component (`/p2p/<peerID>`)
- Check that P2P ports are not blocked by a firewall or already in use
- Nodes wait up to 15 seconds for peers on startup -- check logs for connection progress
- All nodes run DHT in server mode; client mode breaks non-bootstrap discovery

### Consensus not reaching agreement

- Network must have at least 4 nodes for `f=1`
- Byzantine node count must be strictly less than `n/3`
- Primary checks for `2f` connected peers before proposing
- Check logs for view change messages (indicates primary timeout at 15s)

### Non-primary validation hangs

- Non-primary nodes forward requests via broadcast and poll local state
- If primary is unreachable, the request will retry every 5 seconds until the 20-second timeout
- Check that the primary node is running and reachable

### Stale state across nodes

- Anti-entropy runs every 5 seconds -- wait for sync
- State replication broadcasts directly after consensus for fast propagation
- Use `curl http://localhost:8081/api/v1/stats` to compare sequence numbers across nodes
- On startup, each node immediately requests state sync from a random peer

### Deterministic peer ID issues

- Peer IDs are derived from `SHA-256(DTVN_KEY_SEED + nodeID)`
- Default seed is `"dtvn-validator-key-v1"` (development only)
- Set `DTVN_KEY_SEED` to a secret value in production
- If you change node ID generation, update bootstrap addresses in the Makefile

## Makefile Targets

```bash
make help    # Show all available targets
```

| Target                              | Description                                         |
| ----------------------------------- | --------------------------------------------------- |
| `build`                             | Build validator, simulator, and supervisor binaries |
| `clean`                             | Remove bin/, data/, and \*.db files                 |
| `test`                              | Run all tests                                       |
| `test-unit`                         | Unit tests only (fast)                              |
| `test-coverage`                     | Tests with HTML coverage report                     |
| `test-integration`                  | Integration tests                                   |
| `test-integration-single TEST=Name` | Run a single integration test                       |
| `fmt`                               | Format code (`go fmt`)                              |
| `lint`                              | Run linter (`golangci-lint`)                        |
| `tidy`                              | Tidy dependencies (`go mod tidy`)                   |
| `proto`                             | Regenerate protobuf code                            |
| `run-validator`                     | Run a single validator node                         |
| `run-network`                       | Run a 4-node validator network                      |
| `run-simulator`                     | Run the network simulator                           |
| `run-simulator-partition`           | Run simulator with network partitions               |
| `deps`                              | Install dependencies                                |
| `dev-setup`                         | Set up development environment                      |

## License

MIT License -- see [LICENSE](LICENSE) for details.

## References

- [Practical Byzantine Fault Tolerance (Castro & Liskov, 1999)](http://pmg.csail.mit.edu/papers/osdi99.pdf)
- [libp2p Specifications](https://github.com/libp2p/specs)
- [Epidemic Algorithms for Replicated Database Maintenance (Demers et al., 1987)](https://dl.acm.org/doi/10.1145/41840.41841)
- [Vector Clocks](https://en.wikipedia.org/wiki/Vector_clock)
- [Kademlia: A Peer-to-peer Information System Based on the XOR Metric](https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf)
