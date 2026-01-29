# Integration Test Matrix

Quick reference matrix for all integration test scenarios.

## Test Scenarios Summary

| # | Test Name | Category | Duration | Nodes | Byzantine | Description |
|---|-----------|----------|----------|-------|-----------|-------------|
| 1 | `TestSingleTicketValidation` | Functional | 30-60s | 7 | 0 | Single ticket validation and propagation |
| 2 | `TestConcurrentValidation` | Functional | 1-2min | 9 | 0 | 10 tickets validated concurrently |
| 3 | `TestDoubleValidationPrevention` | Functional | 30-45s | 7 | 0 | Prevent same ticket validation twice |
| 4 | `TestSimultaneousDoubleValidation` | Functional | 30-45s | 7 | 0 | Race condition handling |
| 5 | `TestByzantineNodeTolerance` | Byzantine | 1-2min | 10 | 3 | System operates with Byzantine nodes |
| 6 | `TestByzantineLeader` | Byzantine | 1-2min | 7 | 1 | Byzantine node as potential leader |
| 7 | `TestNetworkPartitionRecovery` | Partition | 2-3min | 9 | 0 | 5-phase partition and recovery test |
| 8 | `TestSplitBrainScenario` | Partition | 1-2min | 10 | 0 | 50/50 network split handling |

**Total Suite Duration**: 8-15 minutes

## Detailed Test Matrix

### Functional Tests

#### 1. Single Ticket Validation

| Property | Value |
|----------|-------|
| **Test ID** | FUNC-001 |
| **Function** | `TestSingleTicketValidation` |
| **Nodes** | 7 |
| **Byzantine** | 0 |
| **Duration** | 30-60 seconds |
| **Assertions** | Latency < 1000ms, Consensus 2f+1, State consistency ≥ 95%, Double validation rejected |

**Steps**:
1. Start 7-node cluster
2. Validate ticket on node 1
3. Wait for consensus
4. Verify all nodes validated
5. Attempt double validation

---

#### 2. Concurrent Validation

| Property | Value |
|----------|-------|
| **Test ID** | FUNC-002 |
| **Function** | `TestConcurrentValidation` |
| **Nodes** | 9 |
| **Byzantine** | 0 |
| **Duration** | 1-2 minutes |
| **Tickets** | 10 |
| **Assertions** | All validations succeed, Avg latency < 1000ms, State consistency ≥ 90%, Throughput > 1/sec |

**Steps**:
1. Start 9-node cluster
2. Submit 10 tickets concurrently to different nodes
3. Wait for all consensus
4. Verify state consistency

---

#### 3. Double Validation Prevention

| Property | Value |
|----------|-------|
| **Test ID** | FUNC-003 |
| **Function** | `TestDoubleValidationPrevention` |
| **Nodes** | 7 |
| **Byzantine** | 0 |
| **Duration** | 30-45 seconds |
| **Assertions** | First validation succeeds, Second validation rejected, State consistency ≥ 95% |

**Steps**:
1. Start 7-node cluster
2. Validate ticket on node 1
3. Wait for consensus
4. Attempt second validation on same node
5. Attempt validation on different node

---

#### 4. Simultaneous Double Validation

| Property | Value |
|----------|-------|
| **Test ID** | FUNC-004 |
| **Function** | `TestSimultaneousDoubleValidation` |
| **Nodes** | 7 |
| **Byzantine** | 0 |
| **Duration** | 30-45 seconds |
| **Assertions** | Consensus prevents double validation, State consistency ≥ 90% |

**Steps**:
1. Start 7-node cluster
2. Submit same ticket to two nodes simultaneously
3. Verify consensus achieves single validation

---

### Byzantine Fault Tolerance Tests

#### 5. Byzantine Node Tolerance

| Property | Value |
|----------|-------|
| **Test ID** | BYZ-001 |
| **Function** | `TestByzantineNodeTolerance` |
| **Nodes** | 10 total |
| **Byzantine** | 3 (f=3) |
| **Honest** | 7 |
| **Duration** | 1-2 minutes |
| **Tickets** | 5 |
| **Assertions** | System makes progress, 2f+1 honest nodes agree, Honest consistency maintained |

**Byzantine Tolerance**: Supports up to f = ⌊(n-1)/3⌋ Byzantine nodes

**Steps**:
1. Start 10-node cluster (3 Byzantine)
2. Validate 5 tickets on honest nodes
3. Verify honest nodes reach consensus
4. Check honest node state consistency

---

#### 6. Byzantine Leader

| Property | Value |
|----------|-------|
| **Test ID** | BYZ-002 |
| **Function** | `TestByzantineLeader` |
| **Nodes** | 7 total |
| **Byzantine** | 1 (potential leader) |
| **Duration** | 1-2 minutes |
| **Assertions** | System makes progress, Honest nodes agree |

**Scenario**: Tests view change mechanism when Byzantine node is leader

**Steps**:
1. Start 7-node cluster with Byzantine node
2. Validate ticket (Byzantine may be leader)
3. Wait for consensus (may trigger view change)
4. Verify honest nodes reached agreement

---

### Network Partition Tests

#### 7. Network Partition Recovery

| Property | Value |
|----------|-------|
| **Test ID** | PART-001 |
| **Function** | `TestNetworkPartitionRecovery` |
| **Nodes** | 9 total |
| **Partition** | 6-3 split (majority-minority) |
| **Duration** | 2-3 minutes |
| **Phases** | 5 |
| **Assertions** | Pre-partition consensus, Majority consensus during partition, State reconciliation, Post-recovery consensus, Consistency ≥ 85% |

**5-Phase Test**:

| Phase | Action | Expected Result |
|-------|--------|----------------|
| 1. Pre-partition | Validate ticket | Normal consensus |
| 2. Create partition | Stop 3 nodes | 6-3 split created |
| 3. During partition | Validate in majority | Majority reaches consensus |
| 4. Heal partition | Restart nodes | All nodes rejoin |
| 5. Post-recovery | Validate new ticket | Normal operation resumes |

---

#### 8. Split-Brain Scenario

| Property | Value |
|----------|-------|
| **Test ID** | PART-002 |
| **Function** | `TestSplitBrainScenario` |
| **Nodes** | 10 total |
| **Partition** | 5-5 split (even) |
| **Duration** | 1-2 minutes |
| **Assertions** | No consensus in split partition |

**Scenario**: Tests that system correctly handles insufficient quorum

**Steps**:
1. Start 10-node cluster
2. Create 50/50 split
3. Attempt validation in one partition
4. Verify no consensus (< 2f+1)

---

## Test Configuration Matrix

| Test | Nodes | Byzantine | Latency | Packet Loss | Partition | Timeout |
|------|-------|-----------|---------|-------------|-----------|---------|
| Single Validation | 7 | 0 | 50ms | 0% | No | 5min |
| Concurrent | 9 | 0 | 50ms | 0% | No | 5min |
| Double Prevention | 7 | 0 | 50ms | 0% | No | 5min |
| Simultaneous Double | 7 | 0 | 50ms | 0% | No | 5min |
| Byzantine Tolerance | 10 | 3 | 50ms | 0% | No | 5min |
| Byzantine Leader | 7 | 1 | 50ms | 0% | No | 5min |
| Partition Recovery | 9 | 0 | 50ms | 0% | Yes | 5min |
| Split Brain | 10 | 0 | 50ms | 0% | Yes | 5min |

## Assertion Matrix

| Assertion | FUNC-001 | FUNC-002 | FUNC-003 | FUNC-004 | BYZ-001 | BYZ-002 | PART-001 | PART-002 |
|-----------|----------|----------|----------|----------|---------|---------|----------|----------|
| Validation latency < 1000ms | ✓ | ✓ | ✓ | ✓ | - | - | ✓ | - |
| P95 latency < 2000ms | ✓ | ✓ | - | - | - | - | - | - |
| Consensus 2f+1 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ |
| State consistency ≥ 95% | ✓ | - | ✓ | - | - | - | - | - |
| State consistency ≥ 90% | - | ✓ | - | ✓ | - | - | - | - |
| State consistency ≥ 85% | - | - | - | - | - | - | ✓ | - |
| Double validation rejected | ✓ | - | ✓ | ✓ | - | - | - | - |
| System makes progress | - | - | - | - | ✓ | ✓ | ✓ | - |
| Throughput > 1 ticket/sec | - | ✓ | - | - | - | - | - | - |
| Partition recovery | - | - | - | - | - | - | ✓ | - |
| No consensus without quorum | - | - | - | - | - | - | - | ✓ |

**Legend**:
- ✓ = Assertion checked and must pass
- ✗ = Assertion checked and must fail
- \- = Assertion not applicable

## Performance Targets

### Latency Targets

| Metric | Target | Acceptable | Test Coverage |
|--------|--------|------------|---------------|
| Mean validation latency | < 1000ms | < 1500ms | FUNC-001, FUNC-002, PART-001 |
| P95 validation latency | < 2000ms | < 3000ms | FUNC-001, FUNC-002 |
| P99 validation latency | < 3000ms | < 5000ms | FUNC-002 |
| Consensus time (normal) | 3-5s | < 10s | All functional tests |
| Consensus time (Byzantine) | 5-10s | < 15s | BYZ-001, BYZ-002 |
| Consensus time (post-partition) | 10-15s | < 30s | PART-001 |

### Throughput Targets

| Scenario | Target | Acceptable | Test Coverage |
|----------|--------|------------|---------------|
| Sequential validation | 1-5 tickets/sec | > 0.5 tickets/sec | FUNC-001 |
| Concurrent validation | 5-10 tickets/sec | > 2 tickets/sec | FUNC-002 |
| With Byzantine nodes | 2-5 tickets/sec | > 1 ticket/sec | BYZ-001 |
| During partition (majority) | 1-3 tickets/sec | > 0.5 tickets/sec | PART-001 |

### Consistency Targets

| Condition | Target | Acceptable | Test Coverage |
|-----------|--------|------------|---------------|
| Normal operation | ≥ 95% | ≥ 90% | FUNC-001, FUNC-003 |
| Concurrent load | ≥ 90% | ≥ 85% | FUNC-002, FUNC-004 |
| With Byzantine nodes | ≥ 90% (honest) | ≥ 80% (honest) | BYZ-001, BYZ-002 |
| Post-partition recovery | ≥ 85% | ≥ 75% | PART-001 |

## Test Execution Guidelines

### Resource Requirements

| Test Category | CPU Cores | RAM | Disk | Network |
|---------------|-----------|-----|------|---------|
| Functional | 2-4 | 4GB | 1GB | 100Mbps |
| Byzantine | 4-8 | 8GB | 2GB | 100Mbps |
| Partition | 4-8 | 8GB | 2GB | 100Mbps |
| Full Suite | 8+ | 16GB | 5GB | 1Gbps |

### Execution Order

**Recommended order** (fast → slow):
1. FUNC-003 (30-45s) - Double Validation Prevention
2. FUNC-001 (30-60s) - Single Ticket Validation
3. FUNC-004 (30-45s) - Simultaneous Double Validation
4. FUNC-002 (1-2min) - Concurrent Validation
5. PART-002 (1-2min) - Split Brain
6. BYZ-002 (1-2min) - Byzantine Leader
7. BYZ-001 (1-2min) - Byzantine Tolerance
8. PART-001 (2-3min) - Partition Recovery

### Parallel Execution

**Safe to run in parallel**:
- FUNC-001, FUNC-003, FUNC-004 (different port ranges)

**Must run sequentially**:
- All tests using same port ranges
- Tests modifying system configuration

## Failure Analysis

### Common Failure Patterns

| Failure Type | Possible Causes | Tests Affected | Debug Steps |
|--------------|----------------|----------------|-------------|
| Consensus timeout | Network issues, Byzantine behavior | All | Check logs, verify node health |
| Port conflicts | Previous test cleanup failed | All | Kill processes, clean ports |
| State inconsistency | Race conditions, Byzantine nodes | BYZ-*, PART-* | Increase wait times, check logs |
| Node startup failure | Resource exhaustion, binary missing | All | Check resources, rebuild binary |
| Validation rejection | Duplicate ticket, state conflict | FUNC-* | Verify ticket uniqueness |

### Debugging Commands

```bash
# Check test status
make test-integration-verbose

# Run single test
make test-integration-single TEST=TestName

# Keep artifacts
make test-integration-debug

# Check logs
tail -f test-data/logs/*.log

# Check ports
lsof -i :14000-14010
lsof -i :18000-18010

# Monitor resources
watch -n 1 'ps aux | grep validator'
```

## CI/CD Integration

### GitHub Actions

```yaml
- name: Integration Tests
  run: make test-integration
  timeout-minutes: 45
```

### Expected CI Duration

- **Full Suite**: 8-15 minutes
- **With 2x buffer**: 20-30 minutes
- **Recommended timeout**: 45 minutes

---

**Last Updated**: 2026-01-27
**Test Suite Version**: 1.0.0
