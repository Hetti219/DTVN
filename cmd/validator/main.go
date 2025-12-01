package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Hetti219/distributed-ticket-validation/pkg/api"
	"github.com/Hetti219/distributed-ticket-validation/pkg/consensus"
	"github.com/Hetti219/distributed-ticket-validation/pkg/gossip"
	"github.com/Hetti219/distributed-ticket-validation/pkg/network"
	"github.com/Hetti219/distributed-ticket-validation/pkg/state"
	"github.com/Hetti219/distributed-ticket-validation/pkg/storage"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// ValidatorNode represents a complete validator node
type ValidatorNode struct {
	nodeID       string
	p2pHost      *network.P2PHost
	discovery    *network.Discovery
	gossipEngine *gossip.GossipEngine
	pbftNode     *consensus.PBFTNode
	stateMachine *state.StateMachine
	storage      *storage.Store
	apiServer    *api.Server
	ctx          context.Context
	cancel       context.CancelFunc
}

// Config holds validator configuration
type Config struct {
	NodeID         string
	ListenPort     int
	APIPort        int
	DataDir        string
	BootstrapPeers []string
	IsBootstrap    bool
	IsPrimary      bool
	TotalNodes     int
}

func main() {
	// Parse command line flags
	nodeID := flag.String("id", "node0", "Node identifier")
	listenPort := flag.Int("port", 4001, "P2P listen port")
	apiPort := flag.Int("api-port", 8080, "API server port")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	bootstrapPeers := flag.String("bootstrap", "", "Comma-separated list of bootstrap peers")
	isBootstrap := flag.Bool("bootstrap-node", false, "Run as bootstrap node")
	isPrimary := flag.Bool("primary", false, "Run as primary (leader) node")
	totalNodes := flag.Int("total-nodes", 4, "Total number of validator nodes")
	flag.Parse()

	// Create configuration
	cfg := &Config{
		NodeID:      *nodeID,
		ListenPort:  *listenPort,
		APIPort:     *apiPort,
		DataDir:     *dataDir,
		IsBootstrap: *isBootstrap,
		IsPrimary:   *isPrimary,
		TotalNodes:  *totalNodes,
	}

	// Parse bootstrap peers
	if *bootstrapPeers != "" {
		// Simple parsing - in production, use proper parsing
		cfg.BootstrapPeers = []string{*bootstrapPeers}
	}

	// Create and start validator node
	node, err := NewValidatorNode(cfg)
	if err != nil {
		fmt.Printf("Failed to create validator node: %v\n", err)
		os.Exit(1)
	}

	if err := node.Start(); err != nil {
		fmt.Printf("Failed to start validator node: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Validator node %s started successfully\n", cfg.NodeID)
	fmt.Printf("P2P listening on port %d\n", cfg.ListenPort)
	fmt.Printf("API server on port %d\n", cfg.APIPort)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down validator node...")
	if err := node.Stop(); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}
	fmt.Println("Validator node stopped")
}

// NewValidatorNode creates a new validator node
func NewValidatorNode(cfg *Config) (*ValidatorNode, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Initialize storage
	store, err := storage.NewStore(&storage.Config{
		Path:    fmt.Sprintf("%s/%s.db", cfg.DataDir, cfg.NodeID),
		Timeout: 10 * time.Second,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize P2P host
	p2pHost, err := network.NewP2PHost(ctx, &network.Config{
		ListenPort:    cfg.ListenPort,
		BootstrapMode: cfg.IsBootstrap,
		DataDir:       cfg.DataDir,
	})
	if err != nil {
		store.Close()
		cancel()
		return nil, fmt.Errorf("failed to initialize P2P host: %w", err)
	}

	// Parse bootstrap peers
	var bootstrapPeers []peer.AddrInfo
	for _, peerAddr := range cfg.BootstrapPeers {
		if peerAddr == "" {
			continue
		}
		addr, err := multiaddr.NewMultiaddr(peerAddr)
		if err != nil {
			fmt.Printf("Invalid bootstrap peer address %s: %v\n", peerAddr, err)
			continue
		}
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			fmt.Printf("Failed to parse peer info from %s: %v\n", peerAddr, err)
			continue
		}
		bootstrapPeers = append(bootstrapPeers, *peerInfo)
	}

	// Initialize discovery
	discovery, err := network.NewDiscovery(ctx, p2pHost.Host(), &network.DiscoveryConfig{
		BootstrapPeers: bootstrapPeers,
		IsBootstrap:    cfg.IsBootstrap,
	})
	if err != nil {
		p2pHost.Close()
		store.Close()
		cancel()
		return nil, fmt.Errorf("failed to initialize discovery: %w", err)
	}

	// Initialize gossip engine
	gossipEngine, err := gossip.NewGossipEngine(ctx, &gossip.Config{
		NodeID: cfg.NodeID,
		Fanout: 0, // Auto-calculate
	}, p2pHost)
	if err != nil {
		discovery.Close()
		p2pHost.Close()
		store.Close()
		cancel()
		return nil, fmt.Errorf("failed to initialize gossip engine: %w", err)
	}

	// Initialize state machine
	stateMachine := state.NewStateMachine(cfg.NodeID)

	// Initialize PBFT consensus
	pbftNode, err := consensus.NewPBFTNode(ctx, &consensus.Config{
		NodeID:      cfg.NodeID,
		TotalNodes:  cfg.TotalNodes,
		IsPrimary:   cfg.IsPrimary,
		ViewTimeout: 5 * time.Second,
	}, p2pHost)
	if err != nil {
		discovery.Close()
		p2pHost.Close()
		store.Close()
		cancel()
		return nil, fmt.Errorf("failed to initialize PBFT: %w", err)
	}

	node := &ValidatorNode{
		nodeID:       cfg.NodeID,
		p2pHost:      p2pHost,
		discovery:    discovery,
		gossipEngine: gossipEngine,
		pbftNode:     pbftNode,
		stateMachine: stateMachine,
		storage:      store,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize API server
	apiServer, err := api.NewServer(ctx, &api.Config{
		Address:    "0.0.0.0",
		Port:       cfg.APIPort,
		EnableCORS: true,
	}, node)
	if err != nil {
		node.Stop()
		return nil, fmt.Errorf("failed to initialize API server: %w", err)
	}
	node.apiServer = apiServer

	// Set up handlers
	node.setupHandlers()

	return node, nil
}

// Start starts all node components
func (n *ValidatorNode) Start() error {
	// Start discovery
	if err := n.discovery.Start(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}

	// Start gossip engine
	n.gossipEngine.Start()

	// Start PBFT
	n.pbftNode.Start()

	// Start API server
	if err := n.apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	return nil
}

// Stop stops all node components
func (n *ValidatorNode) Stop() error {
	n.cancel()

	// Stop API server
	if n.apiServer != nil {
		n.apiServer.Close()
	}

	// Stop PBFT
	if n.pbftNode != nil {
		n.pbftNode.Close()
	}

	// Stop gossip engine
	if n.gossipEngine != nil {
		n.gossipEngine.Close()
	}

	// Stop discovery
	if n.discovery != nil {
		n.discovery.Close()
	}

	// Close P2P host
	if n.p2pHost != nil {
		n.p2pHost.Close()
	}

	// Close storage
	if n.storage != nil {
		n.storage.Close()
	}

	return nil
}

// setupHandlers sets up event handlers
func (n *ValidatorNode) setupHandlers() {
	// Register consensus handler
	n.pbftNode.RegisterHandler(func(req *consensus.Request) error {
		// Handle consensus decision
		switch req.Operation {
		case "VALIDATE":
			return n.stateMachine.ValidateTicket(req.TicketID, n.nodeID, req.Data)
		case "CONSUME":
			return n.stateMachine.ConsumeTicket(req.TicketID, n.nodeID)
		case "DISPUTE":
			return n.stateMachine.DisputeTicket(req.TicketID, n.nodeID)
		}
		return nil
	})

	// Register state machine handler
	n.stateMachine.RegisterHandler(func(ticket *state.Ticket, oldState, newState state.TicketState) error {
		// Persist to storage
		record := &storage.TicketRecord{
			ID:          ticket.ID,
			State:       string(ticket.State),
			Data:        ticket.Data,
			ValidatorID: ticket.ValidatorID,
			Timestamp:   ticket.Timestamp,
			VectorClock: ticket.VectorClock.GetAll(),
			Metadata:    ticket.Metadata,
		}
		return n.storage.SaveTicket(record)
	})

	// Register gossip handler
	n.gossipEngine.RegisterHandler(func(msg *gossip.Message) error {
		// Handle gossip message
		// In production, deserialize and process
		return nil
	})

	// Register peer discovery callback
	n.discovery.OnPeerFound(func(peerInfo peer.AddrInfo) {
		fmt.Printf("Discovered new peer: %s\n", peerInfo.ID)
	})
}

// API interface implementation (for api.ValidatorInterface)

func (n *ValidatorNode) ValidateTicket(ticketID string, data []byte) error {
	// Create consensus request
	req := &consensus.Request{
		RequestID: fmt.Sprintf("%s-%d", n.nodeID, time.Now().Unix()),
		TicketID:  ticketID,
		Operation: "VALIDATE",
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	// Submit to PBFT if primary, otherwise forward
	if n.pbftNode.IsPrimary() {
		return n.pbftNode.ProposeRequest(req)
	}

	// Non-primary nodes would forward to primary
	// For now, just validate locally (simplified)
	return n.stateMachine.ValidateTicket(ticketID, n.nodeID, data)
}

func (n *ValidatorNode) ConsumeTicket(ticketID string) error {
	return n.stateMachine.ConsumeTicket(ticketID, n.nodeID)
}

func (n *ValidatorNode) DisputeTicket(ticketID string) error {
	return n.stateMachine.DisputeTicket(ticketID, n.nodeID)
}

func (n *ValidatorNode) GetTicket(ticketID string) (interface{}, error) {
	return n.stateMachine.GetTicket(ticketID)
}

func (n *ValidatorNode) GetAllTickets() ([]interface{}, error) {
	tickets := n.stateMachine.GetAllTickets()
	result := make([]interface{}, len(tickets))
	for i, t := range tickets {
		result[i] = t
	}
	return result, nil
}

func (n *ValidatorNode) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"node_id":       n.nodeID,
		"peer_count":    n.p2pHost.GetPeerCount(),
		"is_primary":    n.pbftNode.IsPrimary(),
		"current_view":  n.pbftNode.GetView(),
		"sequence":      n.pbftNode.GetSequence(),
		"cache_size":    n.gossipEngine.GetCacheSize(),
		"storage_stats": n.storage.GetStats(),
	}
}
