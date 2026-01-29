# Integration Testing Guide

## Quick Reference

```bash
# Run all integration tests
make test-integration

# Run specific test
make test-integration-single TEST=TestSingleTicketValidation

# Run with verbose output
make test-integration-verbose

# Run in parallel (faster)
make test-integration-parallel

# Keep test data for debugging
make test-integration-debug
```

## Test Suite Overview

The DTVN integration test suite provides comprehensive end-to-end testing of the distributed ticket validation system. Tests cover:

- ✅ Single and concurrent ticket validation
- ✅ Double validation prevention
- ✅ Byzantine fault tolerance (f Byzantine nodes in 3f+1 network)
- ✅ Network partition tolerance and recovery
- ✅ State consistency verification
- ✅ Consensus protocol validation

## Test Categories

### 1. Functional Tests
- `TestSingleTicketValidation` - Basic validation flow
- `TestConcurrentValidation` - Multiple simultaneous validations
- `TestDoubleValidationPrevention` - Prevent duplicate validations
- `TestSimultaneousDoubleValidation` - Race condition handling

### 2. Byzantine Fault Tolerance Tests
- `TestByzantineNodeTolerance` - System with f Byzantine nodes
- `TestByzantineLeader` - Byzantine node as leader

### 3. Network Partition Tests
- `TestNetworkPartitionRecovery` - Partition and recovery
- `TestSplitBrainScenario` - 50/50 network split

## Expected Performance

| Metric | Target | Notes |
|--------|--------|-------|
| Validation Latency (mean) | < 1000ms | Normal operation |
| Validation Latency (P95) | < 2000ms | 95th percentile |
| Throughput | > 1 ticket/sec | Single ticket sequential |
| Consensus Time | 3-5 seconds | Normal operation |
| State Consistency | ≥ 95% | After consensus |

## Test Execution Times

| Test | Duration | Resource Usage |
|------|----------|----------------|
| Single Validation | 30-60s | Low |
| Concurrent Validation | 1-2 min | Medium |
| Double Validation | 30-45s | Low |
| Byzantine Tolerance | 1-2 min | Medium |
| Network Partition | 2-3 min | High |
| Full Suite | 8-15 min | High |

## Troubleshooting

### Common Issues

#### 1. Port Already in Use
```bash
# Find and kill process using port
lsof -ti:14001 | xargs kill -9
# Or change ports in config
```

#### 2. Nodes Fail to Start
```bash
# Check validator binary exists
ls -la bin/validator

# Rebuild
make build

# Check system resources
free -h
nproc
```

#### 3. Consensus Timeouts
- Increase `ConsensusWaitTime` in test config
- Check node logs: `cat test-data/logs/node-*.log`
- Verify no Byzantine behavior in honest nodes

#### 4. State Inconsistency
```bash
# Run with debug mode to inspect
make test-integration-debug

# Check individual node states
ls -la test-data/integration/node-*/
```

### Viewing Test Logs

```bash
# List all logs
ls -la test-data/logs/

# View specific node
cat test-data/logs/node-1.log

# Tail during test run
tail -f test-data/logs/node-*.log

# Search for errors
grep -i error test-data/logs/*.log
```

### Debugging Individual Tests

```bash
# Run single test with Go directly
cd test/integration/scenarios
go test -v -run TestSingleTicketValidation -timeout 10m

# With extra logging
go test -v -run TestSingleTicketValidation -timeout 10m 2>&1 | tee test.log

# Keep test data
go test -v -run TestSingleTicketValidation -timeout 10m --no-cleanup
```

## Configuration

Test configuration is defined in [test/integration/config.go](../test/integration/config.go):

```go
// Default configuration
cfg := integration.DefaultTestConfig()
cfg.NumNodes = 9
cfg.ConsensusWaitTime = 5 * time.Second

// Byzantine testing
cfg := integration.ByzantineTestConfig()
// 10 nodes, 3 Byzantine

// Network partition testing
cfg := integration.PartitionTestConfig()
// Partition-enabled configuration
```

## CI/CD Integration

### GitHub Actions

Add to `.github/workflows/integration.yml`:

```yaml
name: Integration Tests

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

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

      - name: Build
        run: make build

      - name: Run Integration Tests
        run: make test-integration

      - name: Upload test logs
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: integration-test-logs
          path: test-data/logs/
          retention-days: 7
```

### GitLab CI

Add to `.gitlab-ci.yml`:

```yaml
integration-test:
  stage: test
  image: golang:1.21
  timeout: 45m
  script:
    - make build
    - make test-integration
  artifacts:
    when: on_failure
    paths:
      - test-data/logs/
    expire_in: 1 week
```

## Best Practices

### Writing New Tests

1. **Use descriptive names**: `TestFeatureName_Scenario`
2. **Add documentation**: Explain test purpose and steps
3. **Set appropriate timeouts**: Based on test complexity
4. **Clean up resources**: Always defer cleanup
5. **Assert expected behavior**: Use clear assertions
6. **Handle failures gracefully**: Provide debug information

Example:

```go
func TestNewFeature_WithCondition(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    cfg := integration.DefaultTestConfig()
    cluster, err := testutil.NewTestCluster(...)
    require.NoError(t, err)

    defer func() {
        cluster.Stop()
        cluster.Cleanup()
    }()

    // Test implementation

    // Assertions with context
    assert.Greater(t, value, threshold,
        "Expected value to exceed threshold")
}
```

### Performance Testing

```go
// Measure latency
start := time.Now()
err := node.ValidateTicket(ticketID, data)
latency := time.Since(start)

assert.Less(t, latency, cfg.MaxLatency,
    "Validation latency exceeded threshold: %v", latency)
```

### State Verification

```go
// Verify state consistency across nodes
consistency, err := cluster.CheckStateConsistency()
require.NoError(t, err)

assert.GreaterOrEqual(t, consistency, 0.95,
    "State consistency: %.2f%%", consistency*100)
```

## Advanced Usage

### Custom Test Scenarios

Create custom scenarios using the test framework:

```go
runner := integration.NewTestRunner(cfg)

runner.AddScenario(integration.TestScenario{
    Name: "Custom Scenario",
    Description: "Tests custom behavior",
    Config: cfg,
    TestFunc: func(cluster *testutil.TestCluster) error {
        // Your test logic
        return nil
    },
    Assertions: []integration.Assertion{
        {
            Name: "Custom Assertion",
            Check: func(cluster *testutil.TestCluster) error {
                // Verification logic
                return nil
            },
            Critical: true,
        },
    },
})

err := runner.RunAll()
runner.SaveResults("custom-results.json")
```

### Network Condition Simulation

Modify test config for specific conditions:

```go
// High latency
cfg := integration.HighLatencyTestConfig()
cfg.NetworkLatency = 500 * time.Millisecond

// Packet loss
cfg := integration.PacketLossTestConfig()
cfg.PacketLossRate = 0.05  // 5% packet loss

// Custom Byzantine setup
cfg.NumNodes = 13
cfg.ByzantineNodes = []int{10, 11, 12}  // Last 3 nodes
```

## Monitoring Test Execution

### Real-time Monitoring

```bash
# Terminal 1: Run tests
make test-integration-verbose

# Terminal 2: Monitor logs
watch -n 1 'tail -20 test-data/logs/node-1.log'

# Terminal 3: Monitor resources
watch -n 1 'ps aux | grep validator | wc -l'
```

### Post-test Analysis

```bash
# Count errors in logs
for log in test-data/logs/*.log; do
    echo "$log: $(grep -c ERROR $log)"
done

# Extract timing information
grep "consensus reached" test-data/logs/*.log

# Check final states
grep "VALIDATED" test-data/logs/*.log | wc -l
```

## Continuous Improvement

### Adding New Tests

1. Identify untested scenario
2. Create test in `test/integration/scenarios/`
3. Add test documentation
4. Run locally: `make test-integration-single TEST=YourTest`
5. Ensure passes consistently (run 3+ times)
6. Add to CI pipeline

### Performance Benchmarking

```bash
# Run tests multiple times
for i in {1..5}; do
    echo "Run $i"
    make test-integration | tee run-$i.log
done

# Compare results
grep "Duration" run-*.log
```

## Support

For issues with integration tests:

1. Check this documentation
2. Review test logs in `test-data/logs/`
3. Run with debug mode: `make test-integration-debug`
4. Check existing issues on GitHub
5. Create new issue with logs and configuration

## Related Documentation

- [Test Suite README](../test/integration/README.md) - Detailed test information
- [Architecture Guide](./ARCHITECTURE.md) - System design
- [Development Guide](../README.md) - General development setup

---

**Last Updated**: 2026-01-27
**Maintained By**: DTVN Development Team
