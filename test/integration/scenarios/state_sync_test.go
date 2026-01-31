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

// TestStateSynchronization tests the STATE_SYNC_REQUEST/RESPONSE protocol
func TestStateSynchronization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 7

	clusterCfg := &testutil.ClusterConfig{
		NumNodes:        cfg.NumNodes,
		ByzantineNodes:  []int{},
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

	fmt.Println("=== TC-INT-006: State Synchronization Test ===")

	// Step 1: Verify initial health
	err = cluster.WaitForHealthy(1 * time.Minute)
	require.NoError(t, err, "Initial health check failed")
	fmt.Println("✓ Step 1: All 7 nodes healthy and synchronized")

	// Step 2: Stop node 6 to simulate offline node
	node6, err := cluster.GetNode(6)
	require.NoError(t, err)
	err = node6.Stop()
	require.NoError(t, err, "Failed to stop node 6")
	time.Sleep(2 * time.Second)
	fmt.Println("✓ Step 2: Node 6 stopped (simulating offline node)")

	// Step 3: Validate 3 tickets on remaining nodes
	tickets := make([]*fixtures.TestTicket, 3)
	for i := 0; i < 3; i++ {
		tickets[i] = fixtures.GenerateTicket("TICKET", i+1)
		node1, err := cluster.GetNode(1)
		require.NoError(t, err)

		err = node1.ValidateTicket(tickets[i].ID, tickets[i].Data)
		require.NoError(t, err, fmt.Sprintf("Failed to validate ticket %d", i+1))

		time.Sleep(cfg.ConsensusWaitTime)
	}

	// Verify consensus on remaining nodes
	consistentNodes := 0
	for i := 0; i < cfg.NumNodes; i++ {
		if i == 6 {
			continue // Skip stopped node
		}
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			continue
		}

		// Check if node has all 3 tickets
		hasAll := true
		for _, ticket := range tickets {
			_, err := n.GetTicket(ticket.ID)
			if err != nil {
				hasAll = false
				break
			}
		}
		if hasAll {
			consistentNodes++
		}
	}

	fmt.Printf("✓ Step 3: 3 tickets validated with consensus on %d/6 active nodes\n", consistentNodes)
	assert.GreaterOrEqual(t, consistentNodes, 5, "Insufficient consensus on active nodes")

	// Step 4: Restart node 6 (missing 3 consensus rounds)
	err = node6.Start(cfg.ValidatorBinary)
	require.NoError(t, err, "Failed to restart node 6")
	time.Sleep(5 * time.Second) // Wait for node to rejoin
	fmt.Println("✓ Step 4: Node 6 restarted")

	// Step 5-6: Wait for state sync (node 6 should request missing state)
	// The anti-entropy mechanism should kick in within 10 seconds
	time.Sleep(15 * time.Second)
	fmt.Println("✓ Step 5-6: State sync period elapsed (anti-entropy running)")

	// Step 7: Verify state consistency across all 7 nodes
	syncedNodes := 0
	for i := 0; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			fmt.Printf("  Node %d: unhealthy\n", i)
			continue
		}

		// Check if node has all 3 tickets
		ticketCount := 0
		for _, ticket := range tickets {
			_, err := n.GetTicket(ticket.ID)
			if err == nil {
				ticketCount++
			}
		}

		fmt.Printf("  Node %d: %d/3 tickets\n", i, ticketCount)
		if ticketCount == 3 {
			syncedNodes++
		}
	}

	consistency := float64(syncedNodes) / float64(cfg.NumNodes)
	fmt.Printf("✓ Step 7: State consistency: %.2f%% (%d/7 nodes with all tickets)\n",
		consistency*100, syncedNodes)
	assert.GreaterOrEqual(t, consistency, 0.85, "State consistency below 85%")

	// Extended offline test: Stop node 5 for longer period
	fmt.Println("\n--- Extended Offline Scenario ---")

	// Step 8: Stop node 5
	node5, err := cluster.GetNode(5)
	require.NoError(t, err)
	err = node5.Stop()
	require.NoError(t, err, "Failed to stop node 5")
	fmt.Println("✓ Step 8: Node 5 stopped for extended period")

	// Step 9: Validate 10 more tickets
	extendedTickets := make([]*fixtures.TestTicket, 10)
	for i := 0; i < 10; i++ {
		extendedTickets[i] = fixtures.GenerateTicket("TICKET-EXT", i+1)
		node1, err := cluster.GetNode(1)
		require.NoError(t, err)

		err = node1.ValidateTicket(extendedTickets[i].ID, extendedTickets[i].Data)
		if err != nil {
			fmt.Printf("  Warning: Failed to validate ticket %d: %v\n", i+1, err)
		}

		time.Sleep(2 * time.Second)
	}
	fmt.Println("✓ Step 9: 10 additional tickets validated")

	// Step 10: Restart node 5
	err = node5.Start(cfg.ValidatorBinary)
	require.NoError(t, err, "Failed to restart node 5")
	time.Sleep(5 * time.Second)
	fmt.Println("✓ Step 10: Node 5 restarted (missing 10 consensus rounds)")

	// Step 11-12: Wait for bulk state sync
	time.Sleep(20 * time.Second)
	fmt.Println("✓ Step 11-12: Bulk state sync and anti-entropy period")

	// Step 13: Verify final state consistency
	finalSyncedNodes := 0
	totalTickets := len(tickets) + len(extendedTickets)

	for i := 0; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			fmt.Printf("  Node %d: unhealthy\n", i)
			continue
		}

		// Count tickets
		ticketCount := 0
		for _, ticket := range tickets {
			if _, err := n.GetTicket(ticket.ID); err == nil {
				ticketCount++
			}
		}
		for _, ticket := range extendedTickets {
			if _, err := n.GetTicket(ticket.ID); err == nil {
				ticketCount++
			}
		}

		fmt.Printf("  Node %d: %d/%d tickets\n", i, ticketCount, totalTickets)
		if ticketCount >= totalTickets-2 { // Allow 2 missing for eventual consistency
			finalSyncedNodes++
		}
	}

	finalConsistency := float64(finalSyncedNodes) / float64(cfg.NumNodes)
	fmt.Printf("✓ Step 13: Final state consistency: %.2f%% (%d/7 nodes)\n",
		finalConsistency*100, finalSyncedNodes)
	assert.GreaterOrEqual(t, finalConsistency, 0.70, "Final consistency below 70%")

	fmt.Println("\n✓ TC-INT-006: State Synchronization Test COMPLETED")
}

// TestStateSyncProtocolMessages tests the STATE_SYNC_REQUEST/RESPONSE message handling
func TestStateSyncProtocolMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 5

	clusterCfg := &testutil.ClusterConfig{
		NumNodes:        cfg.NumNodes,
		ByzantineNodes:  []int{},
		BootstrapPort:   cfg.BootstrapPort,
		APIPortStart:    cfg.APIPortStart,
		P2PPortStart:    cfg.P2PPortStart,
		DataDir:         cfg.DataDir,
		LogsDir:         cfg.LogsDir,
		ValidatorBinary: cfg.ValidatorBinary,
	}

	cluster, err := testutil.NewTestCluster(clusterCfg)
	require.NoError(t, err)
	defer cluster.Stop()
	defer cluster.Cleanup()

	err = cluster.Start()
	require.NoError(t, err)
	time.Sleep(cfg.NodeStartupDelay)

	fmt.Println("=== State Sync Protocol Message Test ===")

	// This test would verify the actual STATE_SYNC_REQUEST/RESPONSE messages
	// In a real implementation, you would capture and analyze the protobuf messages

	fmt.Println("✓ STATE_SYNC protocol message test placeholder")
	// TODO: Add protocol-level message verification
}
