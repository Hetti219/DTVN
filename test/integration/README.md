# DTVN Integration Tests

Comprehensive integration test suite for the Distributed Ticket Validation Network (DTVN).

## Overview

This test suite validates the end-to-end functionality of the DTVN system, including:

- **Single Ticket Validation**: Validates that a single ticket can be validated and propagates to all nodes
- **Concurrent Validation**: Tests multiple tickets being validated simultaneously
- **Double Validation Prevention**: Ensures tickets cannot be validated twice
- **Byzantine Fault Tolerance**: Tests system behavior with Byzantine nodes present
- **Network Partition Recovery**: Validates system recovery from network partitions
- **State Consistency**: Verifies all nodes maintain consistent state

## Prerequisites

- Go 1.21 or higher
- Linux/macOS environment (Windows via WSL2)
- At least 8GB RAM
- 4+ CPU cores recommended

## Quick Start

### Running All Tests

```bash
# From project root
make integration-test

# Or using the script directly
./scripts/run-integration-tests.sh
```

### Running Specific Tests

```bash
# Run only single validation test
./scripts/run-integration-tests.sh --pattern TestSingleTicketValidation

# Run only Byzantine tests
./scripts/run-integration-tests.sh --pattern Byzantine
```

### Running with Options

```bash
# Verbose output
./scripts/run-integration-tests.sh -v

# Parallel execution (faster but more resource-intensive)
./scripts/run-integration-tests.sh -p

# Keep test data for debugging
./scripts/run-integration-tests.sh --no-cleanup

# Custom timeout
./scripts/run-integration-tests.sh --timeout 45m
```

## Test Structure

```
test/integration/
├── config.go                    # Test configuration
├── runner.go                    # Test orchestration
├── README.md                    # This file
├── fixtures/
│   └── tickets.go              # Test data fixtures
├── testutil/
│   ├── node.go                 # Test node management
│   └── cluster.go              # Cluster management
└── scenarios/
    ├── single_validation_test.go
    ├── concurrent_validation_test.go
    ├── double_validation_test.go
    ├── byzantine_test.go
    └── partition_test.go
```

## Test Scenarios

### 1. Single Ticket Validation (`TestSingleTicketValidation`)

**Purpose**: Validates basic ticket validation and gossip propagation

**Steps**:
1. Start 7-node cluster
2. Validate ticket on node 1
3. Wait for consensus
4. Verify all nodes have validated state
5. Attempt double validation (should fail)

**Assertions**:
- Validation latency < 1000ms
- At least 2f+1 nodes validated
- State consistency ≥ 95%
- Double validation rejected

**Duration**: ~30-60 seconds

---

### 2. Concurrent Validation (`TestConcurrentValidation`)

**Purpose**: Tests system performance under concurrent load

**Steps**:
1. Start 9-node cluster
2. Submit 10 tickets concurrently to different nodes
3. Wait for consensus on all tickets
4. Verify state consistency

**Assertions**:
- All validations succeed
- Average latency < 1000ms
- State consistency ≥ 90%
- Throughput > 1 ticket/second

**Duration**: ~1-2 minutes

---

### 3. Double Validation Prevention (`TestDoubleValidationPrevention`)

**Purpose**: Ensures tickets cannot be validated twice

**Steps**:
1. Start 7-node cluster
2. Validate ticket on node 1
3. Wait for consensus
4. Attempt validation on same node (should fail)
5. Attempt validation on different node (should fail)

**Assertions**:
- First validation succeeds
- Second validation rejected
- State remains consistent

**Duration**: ~30-45 seconds

---

### 4. Simultaneous Double Validation (`TestSimultaneousDoubleValidation`)

**Purpose**: Tests race condition handling

**Steps**:
1. Start 7-node cluster
2. Submit same ticket to two nodes simultaneously
3. Wait for consensus
4. Verify only one validation succeeded at consensus level

**Assertions**:
- Consensus prevents double validation
- State consistency ≥ 90%

**Duration**: ~30-45 seconds

---

### 5. Byzantine Node Tolerance (`TestByzantineNodeTolerance`)

**Purpose**: Validates system operates correctly with Byzantine nodes

**Configuration**: 10 nodes, 3 Byzantine (f=3)

**Steps**:
1. Start cluster with 3 Byzantine nodes
2. Validate 5 tickets on honest nodes
3. Wait for consensus
4. Verify honest nodes reached consensus

**Assertions**:
- System continues to make progress
- At least 2f+1 honest nodes agree
- Honest nodes maintain consistency

**Duration**: ~1-2 minutes

---

### 6. Byzantine Leader (`TestByzantineLeader`)

**Purpose**: Tests behavior when Byzantine node is leader

**Configuration**: 7 nodes, 1 Byzantine as potential leader

**Steps**:
1. Start cluster with Byzantine node
2. Validate ticket (Byzantine may be leader)
3. Wait for consensus (view change may occur)
4. Verify honest nodes reached consensus

**Assertions**:
- System makes progress despite Byzantine leader
- Honest nodes agree on state

**Duration**: ~1-2 minutes

---

### 7. Network Partition Recovery (`TestNetworkPartitionRecovery`)

**Purpose**: Tests partition tolerance and recovery

**Configuration**: 9 nodes, 3-node partition

**Phases**:
1. **Pre-partition**: Validate ticket normally
2. **Create partition**: Stop 3 nodes (6-3 split)
3. **During partition**: Validate in majority partition
4. **Heal partition**: Restart stopped nodes
5. **Post-recovery**: Verify state reconciliation and new validations

**Assertions**:
- Majority partition reaches consensus
- State reconciles after heal
- Consistency ≥ 85% post-recovery
- New validations work after recovery

**Duration**: ~2-3 minutes

---

### 8. Split-Brain Scenario (`TestSplitBrainScenario`)

**Purpose**: Tests 50/50 partition handling

**Configuration**: 10 nodes, 5-5 split

**Steps**:
1. Start 10-node cluster
2. Create 50/50 partition
3. Attempt validation in one partition
4. Verify no consensus reached (insufficient quorum)

**Assertions**:
- Neither partition reaches consensus alone
- System waits for quorum

**Duration**: ~1-2 minutes

## Configuration

Test configuration can be customized in [`config.go`](./config.go):

```go
cfg := &TestConfig{
    NumNodes:          9,              // Number of validator nodes
    ByzantineNodes:    []int{7, 8},    // Indices of Byzantine nodes
    BootstrapPort:     14001,          // Bootstrap node P2P port
    APIPortStart:      18000,          // Starting API port
    P2PPortStart:      14000,          // Starting P2P port
    TestTimeout:       5 * time.Minute,
    NodeStartupDelay:  2 * time.Second,
    ConsensusWaitTime: 5 * time.Second,
    NetworkLatency:    50 * time.Millisecond,
    PacketLossRate:    0.0,            // 0-0.5 (0% to 50%)
    EnablePartitions:  false,
    TicketPrefix:      "TEST-TICKET",
    DataDir:           "./test-data/integration",
    LogsDir:           "./test-data/logs",
    ValidatorBinary:   "./bin/validator",
}
```

### Preset Configurations

- **DefaultTestConfig()**: Standard 9-node cluster
- **ByzantineTestConfig()**: 10 nodes with 3 Byzantine
- **PartitionTestConfig()**: Enables partition testing
- **HighLatencyTestConfig()**: 500ms network latency
- **PacketLossTestConfig()**: 5% packet loss

## Debugging Failed Tests

### View Test Logs

```bash
# Logs are saved to test-data/logs/
ls -la test-data/logs/

# View specific node log
cat test-data/logs/node-1.log

# View last 50 lines
tail -50 test-data/logs/node-1.log
```

### Keep Test Data

```bash
# Run without cleanup to inspect state
./scripts/run-integration-tests.sh --no-cleanup

# Inspect database files
ls -la test-data/integration/
```

### Run Single Test with Verbose Output

```bash
cd test/integration/scenarios
go test -v -run TestSingleTicketValidation -timeout 10m
```

### Common Issues

1. **Nodes fail to start**
   - Check if ports are already in use
   - Verify validator binary exists: `ls -la bin/validator`
   - Check system resources (RAM, CPU)

2. **Consensus timeout**
   - Increase `ConsensusWaitTime` in config
   - Check node logs for errors
   - Verify network connectivity between nodes

3. **State inconsistency**
   - May indicate Byzantine behavior or partition issues
   - Check logs for consensus failures
   - Verify quorum calculations

4. **Port conflicts**
   - Change `APIPortStart` and `P2PPortStart` in config
   - Kill existing processes: `pkill -f validator`

## Performance Expectations

### Latency

- **Mean validation latency**: < 1000ms
- **P95 latency**: < 2000ms
- **P99 latency**: < 3000ms

### Throughput

- **Sequential**: 1-5 tickets/second
- **Concurrent**: 5-10 tickets/second (9-node cluster)

### Consensus

- **Normal operation**: 3-5 seconds
- **Byzantine nodes present**: 5-10 seconds
- **After partition recovery**: 10-15 seconds

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  integration:
    runs-on: ubuntu-latest
    timeout-minutes: 45

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run Integration Tests
        run: |
          make integration-test

      - name: Upload test logs
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: test-logs
          path: test-data/logs/
```

## Advanced Usage

### Writing Custom Tests

Create a new test file in `scenarios/`:

```go
package scenarios

import (
    "testing"
    "github.com/Hetti219/DTVN/test/integration"
    "github.com/Hetti219/DTVN/test/integration/testutil"
)

func TestMyCustomScenario(t *testing.T) {
    cfg := integration.DefaultTestConfig()

    clusterCfg := &testutil.ClusterConfig{
        NumNodes:        cfg.NumNodes,
        // ... other config
    }

    cluster, err := testutil.NewTestCluster(clusterCfg)
    require.NoError(t, err)

    defer func() {
        cluster.Stop()
        cluster.Cleanup()
    }()

    // Your test logic here
}
```

### Running Tests Programmatically

```go
runner := integration.NewTestRunner(config)

runner.AddScenario(integration.TestScenario{
    Name: "My Test",
    Description: "Description",
    Config: config,
    TestFunc: func(cluster *testutil.TestCluster) error {
        // Test implementation
        return nil
    },
    Assertions: []integration.Assertion{
        {
            Name: "Check something",
            Check: func(cluster *testutil.TestCluster) error {
                // Assertion logic
                return nil
            },
            Critical: true,
        },
    },
})

err := runner.RunAll()
runner.SaveResults("results.json")
```

## Contributing

When adding new integration tests:

1. Follow existing test patterns
2. Add clear documentation
3. Set appropriate timeouts
4. Clean up resources properly
5. Add assertions for expected behavior
6. Test both success and failure cases

## License

Same as DTVN project license.
