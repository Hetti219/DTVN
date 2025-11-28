package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Simulator simulates a network of validator nodes for testing
type Simulator struct {
	nodeCount        int
	byzantineCount   int
	ticketCount      int
	simulationTime   time.Duration
	networkLatency   time.Duration
	packetLoss       float64
	partitionEnabled bool
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.RWMutex
	stats            *SimulationStats
}

// SimulationStats tracks simulation statistics
type SimulationStats struct {
	TotalTickets       int
	ValidatedTickets   int
	ConsumedTickets    int
	DisputedTickets    int
	ConsensusRounds    int
	SuccessfulRounds   int
	FailedRounds       int
	AverageLatencyMs   float64
	MessagesSent       int64
	MessagesReceived   int64
	PartitionEvents    int
	ByzantineDetected  int
}

// SimulatedNode represents a simulated validator node
type SimulatedNode struct {
	ID              string
	IsByzantine     bool
	IsPartitioned   bool
	MessageQueue    chan SimulatedMessage
	TicketState     map[string]string
	ConsensusLog    []ConsensusEntry
	VectorClock     map[string]int
	mu              sync.RWMutex
}

// SimulatedMessage represents a network message
type SimulatedMessage struct {
	From      string
	To        string
	Type      string
	Payload   interface{}
	Timestamp time.Time
}

// ConsensusEntry represents a consensus log entry
type ConsensusEntry struct {
	Sequence  int
	TicketID  string
	Operation string
	Timestamp time.Time
}

func main() {
	// Parse flags
	nodeCount := flag.Int("nodes", 7, "Number of validator nodes")
	byzantineCount := flag.Int("byzantine", 2, "Number of Byzantine nodes")
	ticketCount := flag.Int("tickets", 100, "Number of tickets to simulate")
	duration := flag.Duration("duration", 60*time.Second, "Simulation duration")
	latency := flag.Duration("latency", 50*time.Millisecond, "Network latency")
	packetLoss := flag.Float64("packet-loss", 0.01, "Packet loss rate (0.0-1.0)")
	enablePartition := flag.Bool("partition", false, "Enable network partitions")
	flag.Parse()

	// Validate Byzantine count (must be < n/3)
	maxByzantine := (*nodeCount - 1) / 3
	if *byzantineCount > maxByzantine {
		fmt.Printf("Error: Byzantine nodes (%d) exceeds maximum allowed (%d) for %d total nodes\n",
			*byzantineCount, maxByzantine, *nodeCount)
		os.Exit(1)
	}

	fmt.Println("=== Distributed Ticket Validation Network Simulator ===")
	fmt.Printf("Total Nodes: %d\n", *nodeCount)
	fmt.Printf("Byzantine Nodes: %d\n", *byzantineCount)
	fmt.Printf("Tickets: %d\n", *ticketCount)
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("Network Latency: %v\n", *latency)
	fmt.Printf("Packet Loss: %.2f%%\n", *packetLoss*100)
	fmt.Printf("Network Partitions: %v\n", *enablePartition)
	fmt.Println()

	// Create simulator
	sim := NewSimulator(&SimulatorConfig{
		NodeCount:        *nodeCount,
		ByzantineCount:   *byzantineCount,
		TicketCount:      *ticketCount,
		SimulationTime:   *duration,
		NetworkLatency:   *latency,
		PacketLoss:       *packetLoss,
		PartitionEnabled: *enablePartition,
	})

	// Run simulation
	if err := sim.Run(); err != nil {
		fmt.Printf("Simulation error: %v\n", err)
		os.Exit(1)
	}

	// Print results
	sim.PrintResults()
}

// SimulatorConfig holds simulator configuration
type SimulatorConfig struct {
	NodeCount        int
	ByzantineCount   int
	TicketCount      int
	SimulationTime   time.Duration
	NetworkLatency   time.Duration
	PacketLoss       float64
	PartitionEnabled bool
}

// NewSimulator creates a new simulator
func NewSimulator(cfg *SimulatorConfig) *Simulator {
	ctx, cancel := context.WithCancel(context.Background())

	return &Simulator{
		nodeCount:        cfg.NodeCount,
		byzantineCount:   cfg.ByzantineCount,
		ticketCount:      cfg.TicketCount,
		simulationTime:   cfg.SimulationTime,
		networkLatency:   cfg.NetworkLatency,
		packetLoss:       cfg.PacketLoss,
		partitionEnabled: cfg.PartitionEnabled,
		ctx:              ctx,
		cancel:           cancel,
		stats: &SimulationStats{
			TotalTickets: cfg.TicketCount,
		},
	}
}

// Run runs the simulation
func (s *Simulator) Run() error {
	fmt.Println("Starting simulation...")

	// Create nodes
	nodes := s.createNodes()

	// Initialize network
	s.initializeNetwork(nodes)

	// Run simulation
	endTime := time.Now().Add(s.simulationTime)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	ticketIndex := 0
	consensusRound := 0

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-sigChan:
			fmt.Println("\nSimulation interrupted")
			return nil
		case <-ticker.C:
			if time.Now().After(endTime) {
				fmt.Println("\nSimulation completed")
				return nil
			}

			// Simulate ticket operations
			if ticketIndex < s.ticketCount {
				ticketID := fmt.Sprintf("ticket-%d", ticketIndex)
				s.simulateTicketValidation(nodes, ticketID)
				ticketIndex++
				consensusRound++
			}

			// Simulate network partition (occasionally)
			if s.partitionEnabled && rand.Float64() < 0.05 {
				s.simulateNetworkPartition(nodes)
			}

			// Update stats
			s.updateStats(nodes, consensusRound)

			// Print progress
			if consensusRound%10 == 0 {
				fmt.Printf("\rProgress: %d/%d tickets | Rounds: %d | Success: %d | Failed: %d",
					ticketIndex, s.ticketCount, s.stats.ConsensusRounds,
					s.stats.SuccessfulRounds, s.stats.FailedRounds)
			}
		}
	}
}

// createNodes creates simulated validator nodes
func (s *Simulator) createNodes() []*SimulatedNode {
	nodes := make([]*SimulatedNode, s.nodeCount)

	// Create nodes
	for i := 0; i < s.nodeCount; i++ {
		isByzantine := i < s.byzantineCount

		nodes[i] = &SimulatedNode{
			ID:           fmt.Sprintf("node-%d", i),
			IsByzantine:  isByzantine,
			MessageQueue: make(chan SimulatedMessage, 100),
			TicketState:  make(map[string]string),
			ConsensusLog: make([]ConsensusEntry, 0),
			VectorClock:  make(map[string]int),
		}

		if isByzantine {
			fmt.Printf("Node %s configured as Byzantine\n", nodes[i].ID)
		}
	}

	return nodes
}

// initializeNetwork initializes the network connections
func (s *Simulator) initializeNetwork(nodes []*SimulatedNode) {
	fmt.Printf("Initializing network with %d nodes...\n", len(nodes))

	// Start message handlers for each node
	for _, node := range nodes {
		go s.handleNodeMessages(node, nodes)
	}
}

// simulateTicketValidation simulates a ticket validation consensus round
func (s *Simulator) simulateTicketValidation(nodes []*SimulatedNode, ticketID string) {
	// Select primary node (node-0 for simplicity)
	primary := nodes[0]

	// Primary proposes ticket validation
	msg := SimulatedMessage{
		From:      primary.ID,
		To:        "all",
		Type:      "VALIDATE",
		Payload:   ticketID,
		Timestamp: time.Now(),
	}

	// Broadcast to all nodes
	s.broadcast(nodes, msg)

	// Simulate consensus delay
	time.Sleep(s.networkLatency * time.Duration(len(nodes)))
}

// broadcast broadcasts a message to all nodes
func (s *Simulator) broadcast(nodes []*SimulatedNode, msg SimulatedMessage) {
	for _, node := range nodes {
		// Simulate packet loss
		if rand.Float64() < s.packetLoss {
			continue
		}

		// Simulate network latency
		go func(n *SimulatedNode) {
			time.Sleep(s.networkLatency)
			select {
			case n.MessageQueue <- msg:
				s.stats.MessagesSent++
			default:
				// Queue full, drop message
			}
		}(node)
	}
}

// handleNodeMessages handles incoming messages for a node
func (s *Simulator) handleNodeMessages(node *SimulatedNode, allNodes []*SimulatedNode) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-node.MessageQueue:
			s.stats.MessagesReceived++

			// Byzantine nodes behave incorrectly
			if node.IsByzantine {
				s.handleByzantineNode(node, msg, allNodes)
				continue
			}

			// Normal nodes process messages correctly
			s.handleHonestNode(node, msg)
		}
	}
}

// handleByzantineNode handles Byzantine node behavior
func (s *Simulator) handleByzantineNode(node *SimulatedNode, msg SimulatedMessage, allNodes []*SimulatedNode) {
	// Byzantine behavior: send conflicting messages, delay responses, etc.
	if rand.Float64() < 0.5 {
		// Send conflicting state
		node.mu.Lock()
		node.TicketState[msg.Payload.(string)] = "INVALID"
		node.mu.Unlock()
	}
	// Sometimes don't respond at all (dropped message)
}

// handleHonestNode handles honest node behavior
func (s *Simulator) handleHonestNode(node *SimulatedNode, msg SimulatedMessage) {
	node.mu.Lock()
	defer node.mu.Unlock()

	switch msg.Type {
	case "VALIDATE":
		ticketID := msg.Payload.(string)
		node.TicketState[ticketID] = "VALIDATED"
		node.ConsensusLog = append(node.ConsensusLog, ConsensusEntry{
			Sequence:  len(node.ConsensusLog),
			TicketID:  ticketID,
			Operation: "VALIDATE",
			Timestamp: time.Now(),
		})
	}
}

// simulateNetworkPartition simulates a network partition
func (s *Simulator) simulateNetworkPartition(nodes []*SimulatedNode) {
	s.stats.PartitionEvents++

	// Partition 1/3 of nodes
	partitionSize := len(nodes) / 3
	for i := 0; i < partitionSize; i++ {
		nodes[i].mu.Lock()
		nodes[i].IsPartitioned = true
		nodes[i].mu.Unlock()
	}

	// Heal partition after a while
	go func() {
		time.Sleep(5 * time.Second)
		for i := 0; i < partitionSize; i++ {
			nodes[i].mu.Lock()
			nodes[i].IsPartitioned = false
			nodes[i].mu.Unlock()
		}
	}()
}

// updateStats updates simulation statistics
func (s *Simulator) updateStats(nodes []*SimulatedNode, round int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.ConsensusRounds = round

	// Check consensus agreement
	if len(nodes) > 0 && len(nodes[0].ConsensusLog) > 0 {
		// Simple check: compare first and last node logs
		if len(nodes[0].ConsensusLog) == len(nodes[len(nodes)-1].ConsensusLog) {
			s.stats.SuccessfulRounds = len(nodes[0].ConsensusLog)
		} else {
			s.stats.FailedRounds++
		}
	}

	// Count validated tickets
	validatedCount := 0
	for _, node := range nodes {
		node.mu.RLock()
		validatedCount += len(node.TicketState)
		node.mu.RUnlock()
	}
	if len(nodes) > 0 {
		s.stats.ValidatedTickets = validatedCount / len(nodes)
	}
}

// PrintResults prints simulation results
func (s *Simulator) PrintResults() {
	fmt.Println("\n\n=== Simulation Results ===")
	fmt.Printf("Total Tickets: %d\n", s.stats.TotalTickets)
	fmt.Printf("Validated Tickets: %d\n", s.stats.ValidatedTickets)
	fmt.Printf("Consensus Rounds: %d\n", s.stats.ConsensusRounds)
	fmt.Printf("Successful Rounds: %d\n", s.stats.SuccessfulRounds)
	fmt.Printf("Failed Rounds: %d\n", s.stats.FailedRounds)
	fmt.Printf("Success Rate: %.2f%%\n",
		float64(s.stats.SuccessfulRounds)/float64(s.stats.ConsensusRounds)*100)
	fmt.Printf("Messages Sent: %d\n", s.stats.MessagesSent)
	fmt.Printf("Messages Received: %d\n", s.stats.MessagesReceived)
	fmt.Printf("Partition Events: %d\n", s.stats.PartitionEvents)
	fmt.Println()
}
