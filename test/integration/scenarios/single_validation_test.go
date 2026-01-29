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

// TestSingleTicketValidation tests that a single ticket can be validated
// and the validation propagates to all nodes
//
// NOTE: This test currently demonstrates that the DTVN validator's PBFT consensus
// has timing issues - the view timeout occurs before consensus completes.
// The validate API returns "success" after proposing, not after consensus.
func TestSingleTicketValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 4 // Use smaller cluster for faster consensus (2f+1 = 3 nodes needed)
	cfg.ConsensusWaitTime = 15 * time.Second // More time for PBFT consensus to complete

	// Create cluster
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

	// Cleanup after test
	defer func() {
		cluster.Stop()
		cluster.Cleanup()
	}()

	// Start cluster
	err = cluster.Start()
	require.NoError(t, err, "Failed to start cluster")

	// Wait for nodes to be healthy
	time.Sleep(cfg.NodeStartupDelay)

	// Wait extra time for peer connections to establish
	fmt.Println("Waiting for peer connections to stabilize...")
	time.Sleep(3 * time.Second)

	// Generate a test ticket
	ticket := fixtures.GenerateTicket(cfg.TicketPrefix, 1)
	fmt.Printf("Testing validation of ticket: %s\n", ticket.ID)

	// Validate ticket on primary node (node 0 - bootstrap)
	node, err := cluster.GetNode(0)
	require.NoError(t, err)
	fmt.Printf("Validating on node %s (primary)\n", node.ID)

	startTime := time.Now()
	err = node.ValidateTicket(ticket.ID, ticket.Data)
	require.NoError(t, err, "Failed to validate ticket")
	validationLatency := time.Since(startTime)

	fmt.Printf("Initial validation took: %v\n", validationLatency)

	// Assert: Validation latency is within acceptable range
	assert.Less(t, validationLatency, cfg.MaxLatency,
		"Validation latency exceeded maximum")

	// Wait for consensus
	err = cluster.WaitForConsensus(ticket.ID, cfg.ConsensusWaitTime)
	require.NoError(t, err, "Failed to reach consensus")

	// Verify ticket is validated on all healthy nodes
	healthyNodes := cluster.GetHealthyNodes()
	fmt.Printf("Verifying ticket on %d healthy nodes\n", len(healthyNodes))

	validatedCount := 0
	for _, n := range healthyNodes {
		ticketData, err := n.GetTicket(ticket.ID)
		if err != nil {
			t.Logf("Node %s: failed to get ticket: %v", n.ID, err)
			continue
		}

		if data, ok := ticketData["data"].(map[string]interface{}); ok {
			if state, ok := data["State"].(string); ok {
				if state == "VALIDATED" {
					validatedCount++
					fmt.Printf("Node %s: ticket state = %s\n", n.ID, state)
				}
			}
		}
	}

	// Assert: At least 2f+1 nodes have validated the ticket
	requiredNodes := (cfg.NumNodes*2)/3 + 1
	assert.GreaterOrEqual(t, validatedCount, requiredNodes,
		"Insufficient nodes validated the ticket")

	// Verify state consistency
	consistency, err := cluster.CheckStateConsistency()
	require.NoError(t, err, "Failed to check state consistency")
	fmt.Printf("State consistency: %.2f%%\n", consistency*100)

	assert.GreaterOrEqual(t, consistency, 0.95,
		"State consistency below threshold")

	// Try to validate the same ticket again (should fail)
	err = node.ValidateTicket(ticket.ID, ticket.Data)
	assert.Error(t, err, "Double validation should fail")

	fmt.Println("✓ Single ticket validation test passed")
}
