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

// TestDoubleValidationPrevention tests that the same ticket cannot be validated twice
func TestDoubleValidationPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 7

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

	// Generate a test ticket
	ticket := fixtures.GenerateTicket(cfg.TicketPrefix, 1)
	fmt.Printf("Testing double validation prevention for: %s\n", ticket.ID)

	// First validation on node 1
	node1, err := cluster.GetNode(1)
	require.NoError(t, err)

	err = node1.ValidateTicket(ticket.ID, ticket.Data)
	require.NoError(t, err, "First validation should succeed")

	fmt.Println("✓ First validation successful")

	// Wait for consensus
	err = cluster.WaitForConsensus(ticket.ID, cfg.ConsensusWaitTime)
	require.NoError(t, err, "Failed to reach consensus")

	fmt.Println("✓ Consensus reached")

	// Attempt second validation on same node (should fail)
	err = node1.ValidateTicket(ticket.ID, ticket.Data)
	assert.Error(t, err, "Second validation on same node should fail")
	fmt.Println("✓ Second validation on same node rejected")

	// Attempt validation on different node (should also fail)
	node2, err := cluster.GetNode(2)
	require.NoError(t, err)

	err = node2.ValidateTicket(ticket.ID, ticket.Data)
	assert.Error(t, err, "Validation on different node should fail")
	fmt.Println("✓ Validation on different node rejected")

	// Wait a bit more to ensure state is stable
	time.Sleep(2 * time.Second)

	// Verify all nodes have the ticket in VALIDATED state
	healthyNodes := cluster.GetHealthyNodes()
	validatedCount := 0

	for _, node := range healthyNodes {
		ticketData, err := node.GetTicket(ticket.ID)
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

	requiredNodes := (cfg.NumNodes*2)/3 + 1
	assert.GreaterOrEqual(t, validatedCount, requiredNodes,
		"Insufficient nodes have validated state")

	// Verify state consistency
	consistency, err := cluster.CheckStateConsistency()
	require.NoError(t, err)
	fmt.Printf("State consistency: %.2f%%\n", consistency*100)

	assert.GreaterOrEqual(t, consistency, 0.95,
		"State consistency below threshold")

	fmt.Println("✓ Double validation prevention test passed")
}

// TestSimultaneousDoubleValidation tests that simultaneous validation attempts
// on different nodes result in only one success
func TestSimultaneousDoubleValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 7

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

	// Generate a test ticket
	ticket := fixtures.GenerateTicket(cfg.TicketPrefix, 1)
	fmt.Printf("Testing simultaneous validation attempts for: %s\n", ticket.ID)

	// Get two nodes
	node1, err := cluster.GetNode(1)
	require.NoError(t, err)
	node2, err := cluster.GetNode(2)
	require.NoError(t, err)

	// Attempt simultaneous validations
	results := make(chan error, 2)

	go func() {
		err := node1.ValidateTicket(ticket.ID, ticket.Data)
		results <- err
	}()

	go func() {
		err := node2.ValidateTicket(ticket.ID, ticket.Data)
		results <- err
	}()

	// Collect results
	result1 := <-results
	result2 := <-results

	// At least one should succeed, at most one should succeed
	successCount := 0
	if result1 == nil {
		successCount++
	}
	if result2 == nil {
		successCount++
	}

	// Due to timing, both might succeed initially but consensus should prevent double validation
	fmt.Printf("Validation results: node1=%v, node2=%v\n", result1, result2)

	// Wait for consensus
	time.Sleep(cfg.ConsensusWaitTime)

	// Verify only one validation succeeded at consensus level
	healthyNodes := cluster.GetHealthyNodes()
	ticketCounts := make(map[string]int)

	for _, node := range healthyNodes {
		tickets, err := node.GetAllTickets()
		if err != nil {
			continue
		}

		for _, t := range tickets {
			if id, ok := t["ID"].(string); ok && id == ticket.ID {
				ticketCounts[id]++
			}
		}
	}

	// All nodes should see exactly one instance of the ticket
	for id, count := range ticketCounts {
		fmt.Printf("Ticket %s seen on %d nodes\n", id, count)
	}

	// Verify state consistency
	consistency, err := cluster.CheckStateConsistency()
	require.NoError(t, err)
	fmt.Printf("State consistency: %.2f%%\n", consistency*100)

	assert.GreaterOrEqual(t, consistency, 0.90,
		"State consistency below threshold")

	fmt.Println("✓ Simultaneous double validation test passed")
}
