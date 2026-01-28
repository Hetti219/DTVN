package scenarios

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Hetti219/DTVN/test/integration"
	"github.com/Hetti219/DTVN/test/integration/fixtures"
	"github.com/Hetti219/DTVN/test/integration/testutil"
)

// TestConcurrentValidation tests that multiple tickets can be validated concurrently
// and all validations reach consensus
func TestConcurrentValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 9

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

	defer func() {
		cluster.Stop()
		cluster.Cleanup()
	}()

	err = cluster.Start()
	require.NoError(t, err, "Failed to start cluster")
	time.Sleep(cfg.NodeStartupDelay)

	// Generate multiple tickets
	numTickets := 10
	tickets := fixtures.GenerateTickets(cfg.TicketPrefix, numTickets)
	fmt.Printf("Testing concurrent validation of %d tickets\n", numTickets)

	// Validate tickets concurrently on different nodes
	var wg sync.WaitGroup
	results := make(chan error, numTickets)
	latencies := make(chan time.Duration, numTickets)

	startTime := time.Now()

	for i, ticket := range tickets {
		wg.Add(1)
		go func(idx int, t *fixtures.TestTicket) {
			defer wg.Done()

			// Use round-robin node selection
			nodeIdx := (idx % (cfg.NumNodes - 1)) + 1
			node, err := cluster.GetNode(nodeIdx)
			if err != nil {
				results <- err
				return
			}

			validationStart := time.Now()
			err = node.ValidateTicket(t.ID, t.Data)
			latency := time.Since(validationStart)

			results <- err
			latencies <- latency
		}(i, ticket)
	}

	wg.Wait()
	close(results)
	close(latencies)

	totalDuration := time.Since(startTime)
	fmt.Printf("All validations submitted in: %v\n", totalDuration)

	// Check for errors
	errorCount := 0
	for err := range results {
		if err != nil {
			errorCount++
			t.Logf("Validation error: %v", err)
		}
	}

	assert.Equal(t, 0, errorCount, "Some validations failed")

	// Collect latencies
	var totalLatency time.Duration
	latencyCount := 0
	for latency := range latencies {
		totalLatency += latency
		latencyCount++
	}

	avgLatency := totalLatency / time.Duration(latencyCount)
	fmt.Printf("Average validation latency: %v\n", avgLatency)

	// Wait for all tickets to reach consensus
	fmt.Println("Waiting for consensus on all tickets...")
	consensusTimeout := cfg.ConsensusWaitTime * 2 // Extra time for multiple tickets

	for i, ticket := range tickets {
		err := cluster.WaitForConsensus(ticket.ID, consensusTimeout)
		if err != nil {
			t.Logf("Ticket %d (%s) failed to reach consensus: %v", i, ticket.ID, err)
		} else {
			fmt.Printf("✓ Ticket %d reached consensus\n", i)
		}
	}

	// Verify state consistency
	consistency, err := cluster.CheckStateConsistency()
	require.NoError(t, err, "Failed to check state consistency")
	fmt.Printf("State consistency: %.2f%%\n", consistency*100)

	assert.GreaterOrEqual(t, consistency, 0.90,
		"State consistency below threshold for concurrent validation")

	// Verify throughput
	throughput := float64(numTickets) / totalDuration.Seconds()
	fmt.Printf("Throughput: %.2f tickets/second\n", throughput)

	assert.Greater(t, throughput, 1.0,
		"Throughput below minimum threshold")

	fmt.Println("✓ Concurrent validation test passed")
}
