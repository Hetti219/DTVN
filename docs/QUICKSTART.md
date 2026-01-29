# Quick Start Guide

## Prerequisites

- Go 1.21+
- Git
- Make (optional but recommended)

## 1. Build the Project

```bash
# Install dependencies
go mod download

# Build all binaries
go build -o bin/validator cmd/validator/main.go
go build -o bin/simulator cmd/simulator/main.go

# Or use Make
make build
```

## 2. Run a Single Validator Node

```bash
./bin/validator \
  -id node0 \
  -port 4001 \
  -api-port 8080 \
  -data-dir ./data/node0 \
  -primary \
  -total-nodes 4
```

The validator will start and listen on:
- P2P: `localhost:4001`
- API: `http://localhost:8080`
- Metrics: `http://localhost:9090/metrics`

## 3. Test the API

### Validate a ticket

```bash
curl -X POST http://localhost:8080/api/v1/tickets/validate \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "TICKET-001", "data": "eyJ0ZXN0IjoidGlja2V0In0="}'
```

### Get ticket status

```bash
curl http://localhost:8080/api/v1/tickets/TICKET-001
```

### Get node stats

```bash
curl http://localhost:8080/api/v1/stats
```

## 4. Run a Full Network (4 Validators)

```bash
make run-network
```

This starts 4 validator nodes:
- node0: primary, P2P=4001, API=8081
- node1: replica, P2P=4002, API=8082
- node2: replica, P2P=4003, API=8083
- node3: replica, P2P=4004, API=8084

## 5. Run the Network Simulator

```bash
# Basic simulation (7 nodes, 2 Byzantine, 100 tickets)
./bin/simulator

# Custom simulation
./bin/simulator \
  -nodes 7 \
  -byzantine 2 \
  -tickets 500 \
  -duration 120s \
  -latency 100ms \
  -packet-loss 0.02 \
  -partition
```

The simulator will output:
- Real-time progress
- Consensus success rate
- Message statistics
- Byzantine detection

## 6. Monitor the Network

### Prometheus Metrics
Visit `http://localhost:9090/metrics` to see raw metrics

Key metrics:
- `consensus_rounds_total` - Total consensus rounds
- `consensus_success_total` - Successful rounds
- `gossip_messages_sent_total` - Gossip messages sent
- `peer_count` - Connected peers
- `tickets_validated_total` - Validated tickets

### Grafana Dashboards
1. Open `http://localhost:3000`
2. Login with `admin/admin`
3. Navigate to dashboards

## 7. Test Byzantine Fault Tolerance

Start a network with 7 nodes (tolerates 2 Byzantine failures):

```bash
# Simulate Byzantine nodes
./bin/simulator \
  -nodes 7 \
  -byzantine 2 \
  -tickets 100 \
  -duration 60s
```

Expected results:
- Consensus still reaches with 2 Byzantine nodes
- System remains operational
- Invalid states are rejected

## 8. Test Network Partitions

```bash
# Simulate with network partitions
./bin/simulator \
  -nodes 7 \
  -byzantine 1 \
  -tickets 100 \
  -partition
```

The simulator will:
- Randomly partition nodes
- Heal partitions automatically
- Show partition events in results

## Troubleshooting

### Build Errors

```bash
# Clean and rebuild
make clean
go mod tidy
make build
```

### Port Already in Use

Change ports in command line arguments:
```bash
./bin/validator -port 5001 -api-port 9080 ...
```

### Peers Not Connecting

1. Check firewall settings
2. Verify bootstrap peer addresses
3. Check logs for connection errors

### Consensus Not Reaching

1. Ensure minimum 4 nodes
2. Verify Byzantine count < n/3
3. Check network connectivity

## Next Steps

- Read [README.md](README.md) for comprehensive documentation
- Review [API documentation](#) for detailed API reference
- Check [architecture guide](#) for system design details
- Explore [configuration options](config/validator.yaml)

## Common Commands

```bash
# Build
make build

# Run single validator
make run-validator

# Run network
make run-network

# Run simulator
make run-simulator

# Run tests
make test

# Clean up
make clean
```

## Support

For issues or questions:
- Open a GitHub issue
- Check the README.md
- Review the implementation guide
