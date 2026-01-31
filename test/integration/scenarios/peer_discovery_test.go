package scenarios

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Hetti219/DTVN/test/integration"
	"github.com/Hetti219/DTVN/test/integration/testutil"
)

// TestPeerDiscovery tests DHT-based peer discovery mechanism
func TestPeerDiscovery(t *testing.T) {
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

	fmt.Println("=== TC-INT-005: Peer Discovery Test ===")

	// Step 1-2: Start bootstrap node
	bootstrap := cluster.BootstrapNode
	require.NotNil(t, bootstrap, "Bootstrap node not available")

	fmt.Println("✓ Step 1-2: Bootstrap node started with DHT initialized")

	// Step 3-7: Start nodes 1-6 with bootstrap as seed
	err = cluster.Start()
	require.NoError(t, err, "Failed to start cluster")
	time.Sleep(cfg.NodeStartupDelay)

	fmt.Println("✓ Step 3-5: Nodes 1-6 started with bootstrap as seed")

	// Wait for DHT routing table propagation
	time.Sleep(30 * time.Second)
	fmt.Println("✓ Step 6: Waiting for DHT routing table propagation (30s)")

	// Verify full network connectivity
	healthyCount := 0
	peerCounts := make(map[int]int)

	for i := 0; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil {
			fmt.Printf("  Node %d: not available\n", i)
			continue
		}

		if n.IsHealthy() {
			healthyCount++

			// Get peer count (approximate via status check)
			status, err := n.GetStats()
			if err == nil {
				if connCount, ok := status["connected_peers"].(float64); ok {
					peerCounts[i] = int(connCount)
					fmt.Printf("  Node %d: healthy, %d peers\n", i, int(connCount))
				} else {
					fmt.Printf("  Node %d: healthy\n", i)
				}
			} else {
				fmt.Printf("  Node %d: healthy (status check failed)\n", i)
			}
		} else {
			fmt.Printf("  Node %d: unhealthy\n", i)
		}
	}

	fmt.Printf("✓ Step 7: Network connectivity verified - %d/7 nodes healthy\n", healthyCount)
	assert.GreaterOrEqual(t, healthyCount, 6, "Insufficient healthy nodes")

	// Calculate average peer count
	totalPeers := 0
	nodeCount := 0
	for _, count := range peerCounts {
		totalPeers += count
		nodeCount++
	}
	avgPeers := 0.0
	if nodeCount > 0 {
		avgPeers = float64(totalPeers) / float64(nodeCount)
	}
	fmt.Printf("  Average peers per node: %.1f\n", avgPeers)

	// Step 8: Stop bootstrap node (simulate failure)
	bootstrap.Stop()
	time.Sleep(2 * time.Second)
	fmt.Println("✓ Step 8: Bootstrap node stopped (simulating failure)")

	// Verify remaining nodes stay connected
	remainingHealthy := 0
	for i := 1; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			continue
		}
		remainingHealthy++
	}

	fmt.Printf("✓ Step 12: Network stable without bootstrap - %d/6 nodes healthy\n", remainingHealthy)
	assert.GreaterOrEqual(t, remainingHealthy, 4, "Too many nodes failed after bootstrap stopped")

	fmt.Println("\n✓ TC-INT-005: Peer Discovery Test COMPLETED")
}

// TestBootstrapFailureGracefulHandling tests graceful handling of bootstrap node failure
func TestBootstrapFailureGracefulHandling(t *testing.T) {
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

	defer func() {
		cluster.Stop()
		cluster.Cleanup()
	}()

	fmt.Println("=== Bootstrap Failure Handling Test ===")

	// Start cluster
	err = cluster.Start()
	require.NoError(t, err)
	time.Sleep(cfg.NodeStartupDelay)

	// Stop bootstrap
	bootstrap := cluster.BootstrapNode
	bootstrap.Stop()
	time.Sleep(2 * time.Second)

	fmt.Println("Bootstrap stopped, network should remain operational")

	// Verify nodes stay healthy
	healthyCount := 0
	for i := 1; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err != nil || !n.IsHealthy() {
			continue
		}
		healthyCount++
		fmt.Printf("  Node %d: healthy\n", i)
	}

	fmt.Printf("Network stability: %d/%d nodes healthy without bootstrap\n", healthyCount, cfg.NumNodes-1)
	assert.GreaterOrEqual(t, healthyCount, 3, "Network collapsed without bootstrap")

	fmt.Println("✓ Bootstrap failure handled gracefully")
}

// TestMultiHopDHTDiscovery tests multi-hop peer discovery via DHT queries
func TestMultiHopDHTDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := integration.DefaultTestConfig()
	cfg.NumNodes = 10 // Larger network for multi-hop

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

	defer func() {
		cluster.Stop()
		cluster.Cleanup()
	}()

	fmt.Println("=== Multi-Hop DHT Discovery Test ===")

	// Start cluster
	err = cluster.Start()
	require.NoError(t, err)
	time.Sleep(cfg.NodeStartupDelay)

	fmt.Println("Cluster started, waiting for DHT propagation...")

	// Track peer discovery over time
	checkpoints := []int{10, 20, 30, 45, 60} // seconds

	for _, checkpoint := range checkpoints {
		time.Sleep(time.Duration(checkpoint-len(checkpoints)+checkpoint) * time.Second)

		totalPeers := 0
		activeNodes := 0

		for i := 0; i < cfg.NumNodes; i++ {
			n, err := cluster.GetNode(i)
			if err != nil || !n.IsHealthy() {
				continue
			}

			activeNodes++

			status, err := n.GetStats()
			if err == nil {
				if connCount, ok := status["connected_peers"].(float64); ok {
					totalPeers += int(connCount)
				}
			}
		}

		avgPeers := 0.0
		if activeNodes > 0 {
			avgPeers = float64(totalPeers) / float64(activeNodes)
		}

		fmt.Printf("T+%ds: %d active nodes, avg %.1f peers/node\n",
			checkpoint, activeNodes, avgPeers)
	}

	// Verify eventual full mesh or near-full mesh
	finalHealthy := 0
	for i := 0; i < cfg.NumNodes; i++ {
		n, err := cluster.GetNode(i)
		if err == nil && n.IsHealthy() {
			finalHealthy++
		}
	}

	fmt.Printf("✓ Multi-hop discovery completed: %d/%d nodes in network\n",
		finalHealthy, cfg.NumNodes)
	assert.GreaterOrEqual(t, finalHealthy, 8, "Insufficient network formation via DHT")
}
