# Distributed Ticket Validation Network (DTVN)

A Byzantine Fault-Tolerant, peer-to-peer ticket validation system using gossip-based consensus protocols.

## Overview

DTVN is a decentralized network for validating and tracking event tickets using:
- **libp2p** for P2P networking
- **Kademlia DHT** for peer discovery
- **Gossip Protocol** for epidemic message dissemination
- **PBFT** (Practical Byzantine Fault Tolerance) for consensus
- **Vector Clocks** for causality tracking
- **BoltDB** for persistent storage

The system can tolerate up to `f` Byzantine (malicious) nodes where `f = (n-1)/3` and `n` is the total number of validator nodes.

## Architecture

### Layer 0: P2P Infrastructure
- libp2p networking with multiple transports (TCP, QUIC, WebSocket)
- NAT traversal and hole punching
- Secure communication with TLS and Noise
- Connection multiplexing

### Layer 1: Peer Discovery
- Kademlia DHT for distributed peer discovery
- Bootstrap nodes for network bootstrapping
- Automatic peer discovery and connection management

### Layer 2: Gossip Protocol
- Epidemic broadcast for message dissemination
- Anti-entropy for state reconciliation
- Bloom filters for message deduplication
- Dynamic fanout adjustment (sqrt(n))

### Layer 3: Consensus (PBFT)
- Three-phase commit protocol (Pre-Prepare, Prepare, Commit)
- View change for leader election
- Checkpointing for garbage collection
- Byzantine fault tolerance (tolerates f < n/3 failures)

### Layer 4: State Machine
- Ticket state transitions (ISSUED → VALIDATED → CONSUMED)
- Vector clocks for causality tracking
- Conflict resolution for concurrent updates
- State merging and synchronization

### Layer 5: Storage
- BoltDB for persistent key-value storage
- Separate buckets for tickets, consensus logs, and checkpoints
- Atomic transactions
- Database backup and compaction

### Layer 6: API
- REST API for ticket operations
- WebSocket for real-time updates
- Prometheus metrics endpoint
- CORS support

## Installation

### Prerequisites
- Go 1.21 or higher
- Docker (optional, for containerized deployment)
- protoc (optional, for protocol buffer generation)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/Hetti219/distributed-ticket-validation.git
cd distributed-ticket-validation

# Install dependencies
go mod download

# Build binaries
make build

# Or build manually
go build -o bin/validator cmd/validator/main.go
go build -o bin/simulator cmd/simulator/main.go
```

## Quick Start

### Running a Single Validator Node

```bash
# Run the primary validator node
./bin/validator \
  -id validator-0 \
  -port 4001 \
  -api-port 8080 \
  -data-dir ./data/validator-0 \
  -primary \
  -total-nodes 4
```

### Running a Network of Validators

Using the Makefile:
```bash
make run-network
```

Or manually (peer IDs are deterministic based on node ID):
```bash
# Terminal 1: Primary node
./bin/validator -id node0 -port 4001 -api-port 8081 -data-dir ./data/node0 -primary -total-nodes 4

# Terminal 2: Validator node 1 (node0's peer ID is deterministic)
./bin/validator -id node1 -port 4002 -api-port 8082 -data-dir ./data/node1 -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"

# Terminal 3: Validator node 2
./bin/validator -id node2 -port 4003 -api-port 8083 -data-dir ./data/node2 -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"

# Terminal 4: Validator node 3
./bin/validator -id node3 -port 4004 -api-port 8084 -data-dir ./data/node3 -total-nodes 4 \
  -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C"
```

### Using Docker Compose

```bash
# Build and start the network (7 validators + monitoring)
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the network
docker-compose down
```

## API Usage

### Validate a Ticket

```bash
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{
    "ticket_id": "TICKET-001",
    "data": "eyJldmVudCI6IkNvbmNlcnQiLCJzZWF0IjoiQTEifQ=="
  }'
```

### Consume a Ticket

```bash
curl -X POST http://localhost:8080/api/v1/tickets/consume \
  -H "Content-Type: application/json" \
  -d '{
    "ticket_id": "TICKET-001"
  }'
```

### Dispute a Ticket

```bash
curl -X POST http://localhost:8080/api/v1/tickets/dispute \
  -H "Content-Type: application/json" \
  -d '{
    "ticket_id": "TICKET-001"
  }'
```

### Get Ticket Status

```bash
curl http://localhost:8080/api/v1/tickets/TICKET-001
```

### Get All Tickets

```bash
curl http://localhost:8080/api/v1/tickets
```

### Get Node Stats

```bash
curl http://localhost:8080/api/v1/stats
```

### WebSocket Connection

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onmessage = (event) => {
  const update = JSON.parse(event.data);
  console.log('Ticket update:', update);
};
```

## Network Simulator

The simulator allows testing of Byzantine fault tolerance, network partitions, and consensus performance.

### Basic Simulation

```bash
./bin/simulator \
  -nodes 7 \
  -byzantine 2 \
  -tickets 100 \
  -duration 60s \
  -latency 50ms \
  -packet-loss 0.01
```

### Simulation with Network Partitions

```bash
./bin/simulator \
  -nodes 7 \
  -byzantine 2 \
  -tickets 100 \
  -duration 120s \
  -latency 100ms \
  -packet-loss 0.05 \
  -partition
```

### Simulator Options

- `-nodes`: Number of validator nodes (default: 7)
- `-byzantine`: Number of Byzantine (malicious) nodes (default: 2, must be < n/3)
- `-tickets`: Number of tickets to simulate (default: 100)
- `-duration`: Simulation duration (default: 60s)
- `-latency`: Network latency (default: 50ms)
- `-packet-loss`: Packet loss rate 0.0-1.0 (default: 0.01)
- `-partition`: Enable network partitions (default: false)

## Monitoring

### Prometheus Metrics

Metrics are exposed at `http://localhost:9090/metrics`:

- **Consensus Metrics**: rounds, success/failure, latency
- **Gossip Metrics**: messages sent/received, cache hits/misses
- **Network Metrics**: peer count, bandwidth
- **Ticket Metrics**: validated, consumed, disputed counts
- **Storage Metrics**: operation counts and latency
- **API Metrics**: request counts and latency

### Grafana Dashboards

Access Grafana at `http://localhost:3000` (admin/admin)

Pre-configured dashboards show:
- Network topology
- Consensus performance
- Ticket throughput
- Node health status

## Testing

### Run Unit Tests

```bash
make test
```

### Run with Coverage

```bash
make test-coverage
```

### Integration Tests

```bash
go test -v ./test/integration/...
```

## Configuration

Edit `config/validator.yaml` to customize:

```yaml
node:
  id: "validator-1"
  data_dir: "./data/validator-1"

network:
  listen_port: 4001
  bootstrap_peers:
    - "/ip4/127.0.0.1/tcp/4000/p2p/..."

consensus:
  total_nodes: 7
  view_timeout: 5s

gossip:
  fanout: 3
  ttl: 10
```

## Byzantine Fault Tolerance

The system tolerates up to `f` Byzantine nodes where:
- `f = (n - 1) / 3`
- Minimum network size: 4 nodes (tolerates 1 Byzantine node)
- Recommended: 7 nodes (tolerates 2 Byzantine nodes)

### Valid Network Sizes

- 4 nodes → tolerates 1 Byzantine node
- 7 nodes → tolerates 2 Byzantine nodes
- 10 nodes → tolerates 3 Byzantine nodes
- 13 nodes → tolerates 4 Byzantine nodes

## Performance

### Benchmarks (7-node network)

- **Consensus Latency**: ~150ms (3 RTT rounds)
- **Throughput**: ~100 tickets/second
- **Gossip Fanout**: √7 ≈ 3 peers
- **Message Overhead**: O(n²) per consensus round

### Scalability

- Peer discovery: O(log n)
- Gossip dissemination: O(n log n)
- PBFT consensus: O(n²) messages
- Storage: O(tickets)

## Project Structure

```
.
├── cmd/
│   ├── validator/          # Validator node binary
│   └── simulator/          # Network simulator
├── pkg/
│   ├── network/            # P2P networking & discovery
│   ├── gossip/             # Gossip protocol
│   ├── consensus/          # PBFT consensus
│   ├── state/              # State machine & vector clocks
│   ├── storage/            # BoltDB storage
│   └── api/                # REST API & WebSocket
├── internal/
│   ├── crypto/             # Cryptographic utilities
│   ├── types/              # Common types
│   └── metrics/            # Prometheus metrics
├── proto/                  # Protocol buffer definitions
├── config/                 # Configuration files
├── test/                   # Tests
├── docker/                 # Docker files
├── Makefile               # Build automation
└── README.md              # This file
```

## Development

### Code Style

```bash
# Format code
make fmt

# Run linter
make lint
```

### Adding New Features

1. Implement in appropriate package (pkg/*)
2. Add tests
3. Update API if needed
4. Update documentation

## Troubleshooting

### Peers Not Connecting

- Check firewall rules
- Verify bootstrap peer addresses
- Ensure ports are not in use

### Consensus Not Reaching

- Verify network size meets minimum (4 nodes)
- Check Byzantine node count (must be < n/3)
- Review logs for errors

### High Latency

- Reduce network latency parameter
- Increase fanout for gossip
- Check system resources

## License

MIT License - see LICENSE file for details

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## References

- [Practical Byzantine Fault Tolerance (PBFT)](http://pmg.csail.mit.edu/papers/osdi99.pdf)
- [libp2p Specifications](https://github.com/libp2p/specs)
- [Epidemic Algorithms for Replicated Database Maintenance](https://dl.acm.org/doi/10.1145/41840.41841)
- [Vector Clocks](https://en.wikipedia.org/wiki/Vector_clock)

## Contact

For questions or issues, please open a GitHub issue.
