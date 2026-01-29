package testutil

import (
	"fmt"
	"sync"
	"time"
)

// TestCluster manages a cluster of test nodes
type TestCluster struct {
	Nodes           []*TestNode
	BootstrapNode   *TestNode
	ValidatorBinary string
	StartTime       time.Time
	mu              sync.RWMutex
}

// ClusterConfig holds configuration for a test cluster
type ClusterConfig struct {
	NumNodes        int
	ByzantineNodes  []int
	BootstrapPort   int
	APIPortStart    int
	P2PPortStart    int
	DataDir         string
	LogsDir         string
	ValidatorBinary string
	StartupDelay    time.Duration
}

// NewTestCluster creates a new test cluster
func NewTestCluster(cfg *ClusterConfig) (*TestCluster, error) {
	cluster := &TestCluster{
		Nodes:           make([]*TestNode, 0, cfg.NumNodes),
		ValidatorBinary: cfg.ValidatorBinary,
	}

	// Create bootstrap node
	bootstrapCfg := &NodeConfig{
		Index:           0,
		APIPort:         cfg.APIPortStart,
		P2PPort:         cfg.BootstrapPort,
		DataDir:         cfg.DataDir,
		LogsDir:         cfg.LogsDir,
		IsBootstrap:     true,
		TotalNodes:      cfg.NumNodes,
		ValidatorBinary: cfg.ValidatorBinary,
	}

	bootstrapNode, err := NewTestNode(bootstrapCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap node: %w", err)
	}

	cluster.BootstrapNode = bootstrapNode
	cluster.Nodes = append(cluster.Nodes, bootstrapNode)

	// Get bootstrap node's peer ID for the bootstrap address
	bootstrapPeerID, err := GetPeerIDForNode("node0")
	if err != nil {
		return nil, fmt.Errorf("failed to get bootstrap peer ID: %w", err)
	}

	// Bootstrap address for other nodes (includes peer ID)
	bootstrapAddr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", cfg.BootstrapPort, bootstrapPeerID.String())

	// Create validator nodes
	// Start P2P ports after bootstrap port to avoid conflicts
	for i := 1; i < cfg.NumNodes; i++ {
		isByzantine := contains(cfg.ByzantineNodes, i)

		nodeCfg := &NodeConfig{
			Index:           i,
			APIPort:         cfg.APIPortStart + i,
			P2PPort:         cfg.BootstrapPort + i, // Use bootstrap port as base to avoid conflicts
			DataDir:         cfg.DataDir,
			LogsDir:         cfg.LogsDir,
			BootstrapAddr:   bootstrapAddr,
			IsByzantine:     isByzantine,
			TotalNodes:      cfg.NumNodes,
			ValidatorBinary: cfg.ValidatorBinary,
		}

		node, err := NewTestNode(nodeCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create node %d: %w", i, err)
		}

		cluster.Nodes = append(cluster.Nodes, node)
	}

	return cluster, nil
}

// Start starts all nodes in the cluster
func (c *TestCluster) Start() error {
	c.StartTime = time.Now()

	// Start bootstrap node first
	fmt.Printf("Starting bootstrap node...\n")
	if err := c.BootstrapNode.Start(c.ValidatorBinary); err != nil {
		return fmt.Errorf("failed to start bootstrap node: %w", err)
	}

	// Wait for bootstrap node to be healthy
	maxWait := 30 * time.Second
	waitStart := time.Now()
	for !c.BootstrapNode.IsHealthy() {
		if time.Since(waitStart) > maxWait {
			return fmt.Errorf("bootstrap node failed to become healthy")
		}
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("Bootstrap node ready\n")

	// Start other nodes
	for i, node := range c.Nodes {
		if i == 0 {
			continue // Skip bootstrap node
		}

		fmt.Printf("Starting node %d...\n", i)
		if err := node.Start(c.ValidatorBinary); err != nil {
			return fmt.Errorf("failed to start node %d: %w", i, err)
		}

		// Small delay between node starts
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for all nodes to be healthy
	return c.WaitForHealthy(2 * time.Minute)
}

// Stop stops all nodes in the cluster
func (c *TestCluster) Stop() error {
	var wg sync.WaitGroup
	errors := make([]error, 0)
	var errMu sync.Mutex

	for _, node := range c.Nodes {
		wg.Add(1)
		go func(n *TestNode) {
			defer wg.Done()
			if err := n.Stop(); err != nil {
				errMu.Lock()
				errors = append(errors, err)
				errMu.Unlock()
			}
		}(node)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping nodes: %v", errors)
	}

	return nil
}

// Cleanup removes all data and log files
func (c *TestCluster) Cleanup() error {
	var wg sync.WaitGroup
	errors := make([]error, 0)
	var errMu sync.Mutex

	for _, node := range c.Nodes {
		wg.Add(1)
		go func(n *TestNode) {
			defer wg.Done()
			if err := n.Cleanup(); err != nil {
				errMu.Lock()
				errors = append(errors, err)
				errMu.Unlock()
			}
		}(node)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("errors cleaning up nodes: %v", errors)
	}

	return nil
}

// WaitForHealthy waits for all nodes to be healthy
func (c *TestCluster) WaitForHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		allHealthy := true
		for i, node := range c.Nodes {
			if !node.IsHealthy() {
				allHealthy = false
				fmt.Printf("Waiting for node %d to be healthy...\n", i)
				break
			}
		}

		if allHealthy {
			fmt.Printf("All nodes are healthy\n")
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for nodes to be healthy")
		}

		time.Sleep(1 * time.Second)
	}
}

// WaitForConsensus waits for consensus to be reached on a ticket
func (c *TestCluster) WaitForConsensus(ticketID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	requiredNodes := (len(c.Nodes) * 2 / 3) + 1 // 2f+1
	fmt.Printf("Waiting for consensus on ticket %s (need %d/%d nodes)\n", ticketID, requiredNodes, len(c.Nodes))

	for {
		validatedCount := 0
		nodeStatuses := make([]string, 0)

		for _, node := range c.Nodes {
			if !node.IsHealthy() {
				nodeStatuses = append(nodeStatuses, fmt.Sprintf("%s:unhealthy", node.ID))
				continue
			}

			ticket, err := node.GetTicket(ticketID)
			if err != nil {
				nodeStatuses = append(nodeStatuses, fmt.Sprintf("%s:err(%v)", node.ID, err))
				continue
			}

			// Check if ticket is in VALIDATED or CONSUMED state
			// API returns: {"success": true, "data": {"ID":..., "State":...}}
			// or: {"success": false, "error": "..."}
			if success, ok := ticket["success"].(bool); ok && !success {
				// Ticket not found on this node
				errMsg := "unknown"
				if e, ok := ticket["error"].(string); ok {
					errMsg = e
				}
				nodeStatuses = append(nodeStatuses, fmt.Sprintf("%s:not-found(%s)", node.ID, errMsg))
				continue
			}

			if data, ok := ticket["data"].(map[string]interface{}); ok {
				if state, ok := data["State"].(string); ok {
					nodeStatuses = append(nodeStatuses, fmt.Sprintf("%s:%s", node.ID, state))
					if state == "VALIDATED" || state == "CONSUMED" {
						validatedCount++
					}
				} else {
					nodeStatuses = append(nodeStatuses, fmt.Sprintf("%s:no-state", node.ID))
				}
			} else {
				nodeStatuses = append(nodeStatuses, fmt.Sprintf("%s:no-data-field", node.ID))
			}
		}

		if validatedCount >= requiredNodes {
			fmt.Printf("Consensus reached: %d/%d nodes validated ticket %s\n",
				validatedCount, len(c.Nodes), ticketID)
			return nil
		}

		if time.Now().After(deadline) {
			fmt.Printf("Node statuses: %v\n", nodeStatuses)
			return fmt.Errorf("timeout waiting for consensus (got %d/%d nodes)",
				validatedCount, requiredNodes)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// GetHealthyNodes returns all healthy nodes
func (c *TestCluster) GetHealthyNodes() []*TestNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	healthy := make([]*TestNode, 0)
	for _, node := range c.Nodes {
		if node.IsHealthy() {
			healthy = append(healthy, node)
		}
	}
	return healthy
}

// GetByzantineNodes returns all Byzantine nodes
func (c *TestCluster) GetByzantineNodes() []*TestNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	byzantine := make([]*TestNode, 0)
	for _, node := range c.Nodes {
		if node.IsByzantine {
			byzantine = append(byzantine, node)
		}
	}
	return byzantine
}

// GetNode returns a node by index
func (c *TestCluster) GetNode(index int) (*TestNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if index < 0 || index >= len(c.Nodes) {
		return nil, fmt.Errorf("invalid node index: %d", index)
	}

	return c.Nodes[index], nil
}

// CheckStateConsistency checks if all healthy nodes have consistent state
func (c *TestCluster) CheckStateConsistency() (float64, error) {
	healthyNodes := c.GetHealthyNodes()
	if len(healthyNodes) == 0 {
		return 0.0, fmt.Errorf("no healthy nodes")
	}

	// Get tickets from all nodes
	nodeTickets := make([]map[string]string, len(healthyNodes))
	for i, node := range healthyNodes {
		tickets, err := node.GetAllTickets()
		if err != nil {
			return 0.0, fmt.Errorf("failed to get tickets from node %s: %w", node.ID, err)
		}

		// Build a map of ticket ID -> state
		ticketStates := make(map[string]string)
		for _, ticket := range tickets {
			if id, ok := ticket["ID"].(string); ok {
				if state, ok := ticket["State"].(string); ok {
					ticketStates[id] = state
				}
			}
		}
		nodeTickets[i] = ticketStates
	}

	// Compare states
	if len(nodeTickets) == 0 {
		return 1.0, nil // No tickets, consistent
	}

	// Use first node as reference
	reference := nodeTickets[0]
	consistentCount := 0
	totalComparisons := 0

	for ticketID, refState := range reference {
		for i := 1; i < len(nodeTickets); i++ {
			totalComparisons++
			if state, exists := nodeTickets[i][ticketID]; exists && state == refState {
				consistentCount++
			}
		}
	}

	if totalComparisons == 0 {
		return 1.0, nil
	}

	consistency := float64(consistentCount) / float64(totalComparisons)
	return consistency, nil
}

// GetClusterStats returns aggregate statistics from all nodes
func (c *TestCluster) GetClusterStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"total_nodes":   len(c.Nodes),
		"healthy_nodes": 0,
		"uptime":        time.Since(c.StartTime).Seconds(),
	}

	totalTickets := 0
	totalConsensusRounds := int64(0)
	totalMessagesSent := int64(0)
	totalMessagesReceived := int64(0)

	for _, node := range c.Nodes {
		if !node.IsHealthy() {
			continue
		}

		stats["healthy_nodes"] = stats["healthy_nodes"].(int) + 1

		nodeStats, err := node.GetStats()
		if err != nil {
			continue
		}

		if tickets, ok := nodeStats["total_tickets"].(float64); ok {
			totalTickets += int(tickets)
		}

		if rounds, ok := nodeStats["consensus_rounds"].(float64); ok {
			totalConsensusRounds += int64(rounds)
		}

		if sent, ok := nodeStats["messages_sent"].(float64); ok {
			totalMessagesSent += int64(sent)
		}

		if received, ok := nodeStats["messages_received"].(float64); ok {
			totalMessagesReceived += int64(received)
		}
	}

	stats["total_tickets"] = totalTickets
	stats["consensus_rounds"] = totalConsensusRounds
	stats["messages_sent"] = totalMessagesSent
	stats["messages_received"] = totalMessagesReceived

	return stats, nil
}

// Helper functions

func contains(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
