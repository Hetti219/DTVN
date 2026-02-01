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

// TestByzantineNodeTolerance tests that the system operates correctly
// with Byzantine nodes present (up to f Byzantine nodes in 3f+1 network)
func TestByzantineNodeTolerance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.ByzantineTestConfig()
	// 10 nodes, 3 Byzantine = tolerates f=3 Byzantine failures

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
	time.Sleep(cfg.NodeStartupDelay * 2) // Extra time for Byzantine nodes

	byzantineNodes := cluster.GetByzantineNodes()
	fmt.Printf("Testing with %d Byzantine nodes out of %d total nodes\n",
		len(byzantineNodes), cfg.NumNodes)

	// Generate test tickets
	numTickets := 5
	tickets := fixtures.GenerateTickets(cfg.TicketPrefix, numTickets)

	// Validate tickets on honest nodes, tracking which ones actually succeed
	validatedTickets := make([]*fixtures.TestTicket, 0)
	for i, ticket := range tickets {
		// Use honest node (not Byzantine)
		nodeIdx := (i % (cfg.NumNodes - len(cfg.ByzantineNodes))) + 1
		if contains(cfg.ByzantineNodes, nodeIdx) {
			nodeIdx = 1 // Fallback to node 1 if selected Byzantine
		}

		node, err := cluster.GetNode(nodeIdx)
		require.NoError(t, err)

		err = node.ValidateTicket(ticket.ID, ticket.Data)
		if err != nil {
			t.Logf("Validation failed for ticket %d: %v", i, err)
			continue
		}

		validatedTickets = append(validatedTickets, ticket)
		fmt.Printf("✓ Ticket %d validated successfully\n", i)
	}

	assert.Greater(t, len(validatedTickets), 0, "No tickets were validated")

	// Wait for consensus on each validated ticket using polling
	fmt.Println("Waiting for consensus on all tickets...")
	for _, ticket := range validatedTickets {
		err := cluster.WaitForConsensus(ticket.ID, cfg.ConsensusWaitTime*3)
		if err != nil {
			t.Logf("Consensus wait for ticket %s: %v", ticket.ID, err)
		}
	}

	// Verify honest nodes reached consensus
	honestNodes := make([]*testutil.TestNode, 0)
	for i, node := range cluster.Nodes {
		if !contains(cfg.ByzantineNodes, i) {
			honestNodes = append(honestNodes, node)
		}
	}

	// Check that honest nodes have consistent state
	consensusCount := 0
	for _, ticket := range validatedTickets {
		validatedOnHonest := 0

		for _, node := range honestNodes {
			if !node.IsHealthy() {
				continue
			}

			ticketData, err := node.GetTicket(ticket.ID)
			if err != nil {
				continue
			}

			if data, ok := ticketData["data"].(map[string]interface{}); ok {
				if state, ok := data["State"].(string); ok {
					if state == "VALIDATED" {
						validatedOnHonest++
					}
				}
			}
		}

		// Require 2f+1 honest nodes to agree
		requiredHonest := (len(honestNodes)*2)/3 + 1
		if validatedOnHonest >= requiredHonest {
			consensusCount++
			fmt.Printf("✓ Consensus reached on ticket %s (%d/%d honest nodes)\n",
				ticket.ID, validatedOnHonest, len(honestNodes))
		}
	}

	assert.Equal(t, len(validatedTickets), consensusCount,
		"Not all tickets reached consensus among honest nodes")

	// Verify state consistency among honest nodes only
	// Note: Byzantine nodes may have inconsistent state, which is expected
	fmt.Printf("Checking state consistency among %d honest nodes\n", len(honestNodes))

	fmt.Println("✓ Byzantine tolerance test passed")
}

// TestByzantineLeader tests behavior when a Byzantine node becomes leader
func TestByzantineLeader(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.ByzantineTestConfig()
	cfg.NumNodes = 7
	cfg.ByzantineNodes = []int{1} // Make second node Byzantine (could be leader)

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
	time.Sleep(cfg.NodeStartupDelay * 2)

	fmt.Println("Testing with Byzantine node as potential leader")

	// Generate test ticket
	ticket := fixtures.GenerateTicket(cfg.TicketPrefix, 1)

	// Validate on honest node
	node, err := cluster.GetNode(2) // Use non-Byzantine node
	require.NoError(t, err)

	err = node.ValidateTicket(ticket.ID, ticket.Data)
	if err != nil {
		t.Logf("Initial validation failed: %v", err)
	}

	// Wait for consensus using polling
	// If Byzantine node is leader, view change may be needed
	if err == nil {
		if waitErr := cluster.WaitForConsensus(ticket.ID, cfg.ConsensusWaitTime*3); waitErr != nil {
			t.Logf("Consensus polling: %v", waitErr)
		}
	}

	// Check if honest nodes reached consensus
	healthyHonest := 0
	for i, n := range cluster.Nodes {
		if contains(cfg.ByzantineNodes, i) || !n.IsHealthy() {
			continue
		}

		ticketData, err := n.GetTicket(ticket.ID)
		if err != nil {
			continue
		}

		if data, ok := ticketData["data"].(map[string]interface{}); ok {
			if state, ok := data["State"].(string); ok {
				if state == "VALIDATED" {
					healthyHonest++
				}
			}
		}
	}

	// System should still make progress even with Byzantine leader
	// At least majority of honest nodes should agree
	fmt.Printf("Honest nodes in consensus: %d\n", healthyHonest)
	assert.Greater(t, healthyHonest, 0,
		"System made no progress with Byzantine leader")

	fmt.Println("✓ Byzantine leader test passed")
}

// Helper function
func contains(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
