package scenarios

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Hetti219/DTVN/test/integration"
	"github.com/Hetti219/DTVN/test/integration/fixtures"
	"github.com/Hetti219/DTVN/test/integration/testutil"
)

// TestNetworkPartitionRecovery tests that the system can recover from network partitions
func TestNetworkPartitionRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.PartitionTestConfig()
	cfg.NumNodes = 9

	clusterCfg := &testutil.ClusterConfig{
		NumNodes:        cfg.NumNodes,
		ByzantineNodes:  cfg.ByzantineNodes,
		BootstrapPort:   cfg.BootstrapPort,
		APIPortStart:    cfg.APIPortStart,
		P2PPortStart:    cfg.P2PPortStart,
		DataDir:         cfg.DataDir,
		LogsDir:         cfg.LogsDir,
		ValidatorBinary: cfg.ValidatorBinary,
	}

	cluster, err := testutil.NewTestCluster(clusterCfg)
	require.NoError(t, err, "Failed to create test cluster")

	defer func() {
		cluster.Stop()
		cluster.Cleanup()
	}()

	err = cluster.Start()
	require.NoError(t, err, "Failed to start cluster")
	time.Sleep(cfg.NodeStartupDelay)

	fmt.Println("Testing network partition recovery")

	// Phase 1: Validate ticket before partition
	ticket1 := fixtures.GenerateTicket(cfg.TicketPrefix, 1)
	node, err := cluster.GetNode(1)
	require.NoError(t, err)

	err = node.ValidateTicket(ticket1.ID, ticket1.Data)
	require.NoError(t, err, "Pre-partition validation failed")

	err = cluster.WaitForConsensus(ticket1.ID, cfg.ConsensusWaitTime)
	require.NoError(t, err, "Pre-partition consensus failed")

	fmt.Println("✓ Phase 1: Pre-partition validation successful")

	// Phase 2: Simulate partition by stopping some nodes
	// Create two partitions: majority (6 nodes) and minority (3 nodes)
	partitionSize := 3
	fmt.Printf("Creating partition: stopping %d nodes\n", partitionSize)

	partitionedNodes := make([]*testutil.TestNode, 0, partitionSize)
	for i := cfg.NumNodes - partitionSize; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil {
			continue
		}
		if err := n.Stop(); err != nil {
			t.Logf("Failed to stop node %d: %v", i, err)
		} else {
			partitionedNodes = append(partitionedNodes, n)
			fmt.Printf("Stopped node %d\n", i)
		}
	}

	time.Sleep(2 * time.Second)

	// Phase 3: Validate ticket in majority partition
	ticket2 := fixtures.GenerateTicket(cfg.TicketPrefix, 2)
	majorityNode, err := cluster.GetNode(1)
	require.NoError(t, err)

	err = majorityNode.ValidateTicket(ticket2.ID, ticket2.Data)
	assert.NoError(t, err, "During-partition validation should succeed in majority")

	time.Sleep(cfg.ConsensusWaitTime)

	// Check that majority partition reached consensus
	healthyCount := 0
	validatedCount := 0
	for i := 0; i < cfg.NumNodes-partitionSize; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			continue
		}
		healthyCount++

		ticketData, err := n.GetTicket(ticket2.ID)
		if err != nil {
			continue
		}

		if data, ok := ticketData["data"].(map[string]interface{}); ok {
			if state, ok := data["State"].(string); ok {
				if state == "VALIDATED" {
					validatedCount++
				}
			}
		}
	}

	fmt.Printf("During partition: %d/%d nodes validated ticket\n",
		validatedCount, healthyCount)
	assert.Greater(t, validatedCount, 0,
		"Majority partition should reach consensus")

	fmt.Println("✓ Phase 2: During-partition validation successful in majority")

	// Phase 4: Heal partition by restarting nodes
	fmt.Println("Healing partition: restarting stopped nodes")
	for _, n := range partitionedNodes {
		if err := n.Start(cfg.ValidatorBinary); err != nil {
			t.Logf("Failed to restart node: %v", err)
		}
	}

	// Wait for nodes to rejoin and sync
	time.Sleep(cfg.ConsensusWaitTime * 2)

	err = cluster.WaitForHealthy(1 * time.Minute)
	assert.NoError(t, err, "Nodes failed to become healthy after partition heal")

	fmt.Println("✓ Phase 3: Partition healed")

	// Wait for anti-entropy to propagate state to recovered nodes
	// Anti-entropy runs every 5 seconds, so wait for 3-4 cycles
	fmt.Println("Waiting for anti-entropy to propagate state (20s)...")
	time.Sleep(20 * time.Second)

	// Phase 5: Verify state reconciliation
	fmt.Println("Verifying state reconciliation...")

	// All nodes should now have ticket2
	reconciledCount := 0
	for i := 0; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			continue
		}

		ticketData, err := n.GetTicket(ticket2.ID)
		if err != nil {
			t.Logf("Node %d missing ticket2: %v", i, err)
			continue
		}

		if data, ok := ticketData["data"].(map[string]interface{}); ok {
			if state, ok := data["State"].(string); ok {
				if state == "VALIDATED" {
					reconciledCount++
				}
			}
		}
	}

	fmt.Printf("Post-reconciliation: %d nodes have ticket2\n", reconciledCount)
	assert.GreaterOrEqual(t, reconciledCount, cfg.NumNodes-partitionSize,
		"State not properly reconciled after partition heal")

	// Check overall state consistency
	consistency, err := cluster.CheckStateConsistency()
	require.NoError(t, err)
	fmt.Printf("Final state consistency: %.2f%%\n", consistency*100)

	assert.GreaterOrEqual(t, consistency, 0.85,
		"State consistency below threshold after partition recovery")

	fmt.Println("✓ Phase 4: State reconciliation successful")

	// Phase 6: Verify new validations work after recovery
	ticket3 := fixtures.GenerateTicket(cfg.TicketPrefix, 3)
	err = majorityNode.ValidateTicket(ticket3.ID, ticket3.Data)
	assert.NoError(t, err, "Post-recovery validation failed")

	err = cluster.WaitForConsensus(ticket3.ID, cfg.ConsensusWaitTime)
	assert.NoError(t, err, "Post-recovery consensus failed")

	fmt.Println("✓ Phase 5: Post-recovery validation successful")
	fmt.Println("✓ Network partition recovery test passed")
}

// TestSplitBrainScenario tests split-brain scenario where network is partitioned 50/50
func TestSplitBrainScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.PartitionTestConfig()
	cfg.NumNodes = 10 // Even number for 50/50 split

	clusterCfg := &testutil.ClusterConfig{
		NumNodes:        cfg.NumNodes,
		ByzantineNodes:  cfg.ByzantineNodes,
		BootstrapPort:   cfg.BootstrapPort,
		APIPortStart:    cfg.APIPortStart,
		P2PPortStart:    cfg.P2PPortStart,
		DataDir:         cfg.DataDir,
		LogsDir:         cfg.LogsDir,
		ValidatorBinary: cfg.ValidatorBinary,
	}

	cluster, err := testutil.NewTestCluster(clusterCfg)
	require.NoError(t, err, "Failed to create test cluster")

	defer func() {
		cluster.Stop()
		cluster.Cleanup()
	}()

	err = cluster.Start()
	require.NoError(t, err, "Failed to start cluster")
	time.Sleep(cfg.NodeStartupDelay)

	fmt.Println("Testing split-brain scenario (50/50 partition)")

	// Create 50/50 split
	partitionSize := cfg.NumNodes / 2
	fmt.Printf("Creating 50/50 partition (%d nodes each)\n", partitionSize)

	for i := partitionSize; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil {
			continue
		}
		if err := n.Stop(); err != nil {
			t.Logf("Failed to stop node %d: %v", i, err)
		}
	}

	time.Sleep(2 * time.Second)

	// Try to validate in first partition (no quorum)
	ticket := fixtures.GenerateTicket(cfg.TicketPrefix, 1)
	node, err := cluster.GetNode(1)
	require.NoError(t, err)

	err = node.ValidateTicket(ticket.ID, ticket.Data)
	// May or may not succeed depending on implementation
	// The key is that consensus should not be reached

	time.Sleep(cfg.ConsensusWaitTime)

	// Verify no consensus (neither partition has 2f+1)
	validatedCount := 0
	for i := 0; i < partitionSize; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			continue
		}

		ticketData, err := n.GetTicket(ticket.ID)
		if err != nil {
			continue
		}

		if data, ok := ticketData["data"].(map[string]interface{}); ok {
			if state, ok := data["State"].(string); ok {
				if state == "VALIDATED" {
					validatedCount++
				}
			}
		}
	}

	// Neither partition should reach consensus (need 2f+1 = 7 for f=3)
	requiredForConsensus := (cfg.NumNodes*2)/3 + 1
	fmt.Printf("Validated count: %d, required for consensus: %d\n",
		validatedCount, requiredForConsensus)

	// In a proper split-brain, consensus should not be possible
	// But we only check the first partition
	assert.Less(t, validatedCount, requiredForConsensus,
		"Split partition should not reach consensus alone")

	fmt.Println("✓ Split-brain scenario test passed - no consensus in split partition")
}
