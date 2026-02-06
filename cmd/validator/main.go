package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Hetti219/DTVN/pkg/api"
	"github.com/Hetti219/DTVN/pkg/consensus"
	"github.com/Hetti219/DTVN/pkg/gossip"
	"github.com/Hetti219/DTVN/pkg/network"
	"github.com/Hetti219/DTVN/pkg/state"
	"github.com/Hetti219/DTVN/pkg/storage"
	pb "github.com/Hetti219/DTVN/proto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"
)

// ValidatorNode represents a complete validator node
type ValidatorNode struct {
	nodeID       string
	totalNodes   int
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

	// Parse bootstrap peers (comma-separated)
	if *bootstrapPeers != "" {
		// Split by comma to allow multiple bootstrap peers
		peers := strings.Split(*bootstrapPeers, ",")
		for _, p := range peers {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				cfg.BootstrapPeers = append(cfg.BootstrapPeers, trimmed)
			}
		}
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

	fmt.Printf("✅ Validator node %s started successfully\n", cfg.NodeID)
	fmt.Printf("P2P listening on port %d\n", cfg.ListenPort)
	fmt.Printf("API server on port %d\n", cfg.APIPort)

	// Print full multiaddr with peer ID for bootstrapping other nodes
	fmt.Println("\n📋 Node Multiaddresses (use these for -bootstrap flag):")
	for _, addr := range node.p2pHost.Addrs() {
		fmt.Printf("   %s/p2p/%s\n", addr, node.p2pHost.ID())
	}

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

	// Initialize P2P host with deterministic key based on node ID
	p2pHost, err := network.NewP2PHost(ctx, &network.Config{
		ListenPort:    cfg.ListenPort,
		BootstrapMode: cfg.IsBootstrap,
		DataDir:       cfg.DataDir,
		NodeID:        cfg.NodeID, // Enable deterministic peer ID
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
			fmt.Printf("❌ Invalid bootstrap peer address %s: %v\n", peerAddr, err)
			continue
		}

		// Try to parse as full p2p multiaddr with peer ID
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			// If it fails, the address might be missing the peer ID component
			// Try to extract transport protocols and use peer discovery instead
			fmt.Printf("⚠️  Bootstrap address %s is missing peer ID component\n", peerAddr)
			fmt.Printf("    Attempting to connect using transport address only...\n")

			// Create a temporary AddrInfo with just the multiaddr, no peer ID
			// We'll discover the peer ID when we connect
			bootstrapPeers = append(bootstrapPeers, peer.AddrInfo{
				ID:    "", // Empty peer ID - will be resolved on connection
				Addrs: []multiaddr.Multiaddr{addr},
			})
			continue
		}

		fmt.Printf("✅ Parsed bootstrap peer: %s at %v\n", peerInfo.ID, peerInfo.Addrs)
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
	// ViewTimeout of 15 seconds allows sufficient time for PBFT three-phase commit
	// to complete across network latency. 5 seconds was too aggressive and caused
	// premature view changes before PREPARE votes could be collected.
	pbftNode, err := consensus.NewPBFTNode(ctx, &consensus.Config{
		NodeID:      cfg.NodeID,
		TotalNodes:  cfg.TotalNodes,
		IsPrimary:   cfg.IsPrimary,
		ViewTimeout: 15 * time.Second,
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
		totalNodes:   cfg.TotalNodes,
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

	// Set up peer connection callback for state synchronization
	node.setupPeerCallbacks()

	return node, nil
}

// Start starts all node components
func (n *ValidatorNode) Start() error {
	// Start discovery
	if err := n.discovery.Start(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}

	// Wait for initial peer connections (if multi-node setup)
	expectedPeers := n.totalNodes - 1 // All nodes except self
	if expectedPeers > 0 {
		fmt.Printf("\n⏳ Waiting for peer connections (expected %d peers)...\n", expectedPeers)

		// Wait up to 15 seconds, checking every second
		maxWaitTime := 15 * time.Second
		checkInterval := 1 * time.Second
		startTime := time.Now()

		for time.Since(startTime) < maxWaitTime {
			connectedPeers := n.p2pHost.GetPeerCount()

			if connectedPeers >= expectedPeers {
				fmt.Printf("✅ All peers connected (%d/%d) - ready to accept requests\n\n", connectedPeers, expectedPeers)
				break
			}

			// Log progress every 3 seconds
			elapsed := time.Since(startTime)
			if int(elapsed.Seconds())%3 == 0 && elapsed > 0 {
				fmt.Printf("   Connected to %d/%d peers (waiting %ds)...\n", connectedPeers, expectedPeers, int(elapsed.Seconds()))
			}

			time.Sleep(checkInterval)
		}

		// Final check
		finalPeerCount := n.p2pHost.GetPeerCount()
		if finalPeerCount == 0 {
			fmt.Printf("⚠️  WARNING: No peers connected after %ds!\n", int(maxWaitTime.Seconds()))
			fmt.Printf("⚠️  Node will start but consensus may fail until peers connect.\n")
			fmt.Printf("⚠️  Check:\n")
			fmt.Printf("     1. Other nodes are running\n")
			fmt.Printf("     2. Bootstrap address is correct (must include peer ID)\n")
			fmt.Printf("     3. Network connectivity between nodes\n\n")
		} else if finalPeerCount < expectedPeers {
			fmt.Printf("⚠️  WARNING: Only %d/%d peers connected after %ds\n", finalPeerCount, expectedPeers, int(maxWaitTime.Seconds()))
			fmt.Printf("⚠️  Continuing startup, but consensus requires at least %d nodes for quorum\n\n", 2*((expectedPeers+1)/3)+1)
		}
	} else {
		fmt.Printf("✅ Single-node mode - no peers expected\n\n")
	}

	// Load persisted tickets into state machine before starting network components
	n.loadTicketsFromStore()

	// Start gossip engine
	n.gossipEngine.Start()

	// Start PBFT
	n.pbftNode.Start()

	// Start API server
	if err := n.apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	// Start periodic anti-entropy mechanism for state synchronization
	// This ensures eventual consistency by periodically syncing state with random peers
	go n.runAntiEntropy()

	// Immediately request state sync from a peer on startup to recover
	// any state missed during downtime (e.g., after partition recovery)
	go func() {
		time.Sleep(1 * time.Second) // Brief delay to let connections settle
		n.performAntiEntropySync()
	}()

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

// runAntiEntropy runs periodic state synchronization with random peers
// This implements an anti-entropy protocol to ensure eventual consistency
// by periodically exchanging state with other nodes in the network.
func (n *ValidatorNode) runAntiEntropy() {
	// Anti-entropy interval: sync with a random peer every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	fmt.Printf("AntiEntropy: Started periodic state synchronization (interval: 5s)\n")

	for {
		select {
		case <-n.ctx.Done():
			fmt.Printf("AntiEntropy: Stopping periodic state synchronization\n")
			return
		case <-ticker.C:
			n.performAntiEntropySync()
		}
	}
}

// performAntiEntropySync performs a single anti-entropy synchronization round
func (n *ValidatorNode) performAntiEntropySync() {
	// Get connected peers
	peers := n.p2pHost.GetConnectedPeers()
	if len(peers) == 0 {
		return // No peers to sync with
	}

	// Pick a random peer for anti-entropy exchange
	// Using a simple random selection based on current time
	randomIndex := time.Now().UnixNano() % int64(len(peers))
	selectedPeer := peers[randomIndex]

	// Request state sync from the selected peer
	n.requestStateSync(selectedPeer.ID)
}

// setupHandlers sets up event handlers
func (n *ValidatorNode) setupHandlers() {
	// Create message router
	router := network.NewMessageRouter()

	// Register PBFT handler - routes network messages to consensus layer
	router.RegisterPBFTHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
		return n.pbftNode.HandleNetworkMessage(int32(msgType), payload)
	})

	// Register state handler - enables state synchronization via gossip
	router.RegisterStateHandler(func(update *pb.StateUpdate) error {
		// Convert protobuf tickets to state.Ticket format
		tickets := make([]*state.Ticket, len(update.Tickets))
		for i, ts := range update.Tickets {
			// Convert protobuf state to internal state
			var ticketState state.TicketState
			switch ts.State {
			case pb.State_ISSUED:
				ticketState = state.StateIssued
			case pb.State_PENDING:
				ticketState = state.StatePending
			case pb.State_VALIDATED:
				ticketState = state.StateValidated
			case pb.State_CONSUMED:
				ticketState = state.StateConsumed
			case pb.State_DISPUTED:
				ticketState = state.StateDisputed
			}

			// Convert vector clock
			vc := state.NewVectorClock(n.nodeID)
			if ts.VectorClock != nil {
				for k, v := range ts.VectorClock.Clocks {
					vc.Update(k, v)
				}
			}

			tickets[i] = &state.Ticket{
				ID:          ts.TicketId,
				State:       ticketState,
				ValidatorID: ts.ValidatorId,
				Timestamp:   ts.LastUpdated,
				VectorClock: vc,
				Data:        ts.Metadata, // Use metadata field for data
				Metadata:    make(map[string]string),
			}
		}

		// Merge state from remote node (FIXES unused MergeState)
		if err := n.stateMachine.MergeState(tickets); err != nil {
			return err
		}
		// Persist merged tickets to BoltDB so state survives restarts
		for _, t := range tickets {
			n.persistTicket(t.ID)
		}
		return nil
	})

	// Set router on P2P host
	n.p2pHost.SetRouter(router)

	// Register state publisher - publishes updates via gossip
	n.stateMachine.RegisterPublisher(func(ticketID string, ticketState state.TicketState, validatorID string, timestamp int64) error {
		// Convert internal state to protobuf
		var protoState pb.State
		switch ticketState {
		case state.StateIssued:
			protoState = pb.State_ISSUED
		case state.StatePending:
			protoState = pb.State_PENDING
		case state.StateValidated:
			protoState = pb.State_VALIDATED
		case state.StateConsumed:
			protoState = pb.State_CONSUMED
		case state.StateDisputed:
			protoState = pb.State_DISPUTED
		}

		// Create state update message
		update := &pb.StateUpdate{
			Tickets: []*pb.TicketState{{
				TicketId:    ticketID,
				State:       protoState,
				ValidatorId: validatorID,
				LastUpdated: timestamp,
			}},
			NodeId: n.nodeID,
		}

		// Serialize state update
		payload, err := proto.Marshal(update)
		if err != nil {
			return fmt.Errorf("failed to marshal state update: %w", err)
		}

		// Wrap in ValidatorMessage
		msg := &pb.ValidatorMessage{
			Type:      pb.ValidatorMessage_STATE_UPDATE,
			Payload:   payload,
			SenderId:  n.nodeID,
			Timestamp: time.Now().Unix(),
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal ValidatorMessage: %w", err)
		}

		// Publish via gossip
		return n.gossipEngine.Publish(data)
	})

	// Register consensus handler
	n.pbftNode.RegisterHandler(func(req *consensus.Request) error {
		// Handle consensus decision
		var err error
		switch req.Operation {
		case "VALIDATE":
			err = n.stateMachine.ValidateTicket(req.TicketID, n.nodeID, req.Data)
		case "CONSUME":
			err = n.stateMachine.ConsumeTicket(req.TicketID, n.nodeID)
		case "DISPUTE":
			err = n.stateMachine.DisputeTicket(req.TicketID, n.nodeID)
		}
		if err != nil {
			return err
		}
		// Persist ticket to BoltDB so state survives node restarts
		n.persistTicket(req.TicketID)
		return nil
	})

	// Register state replicator for reliable state propagation after consensus commits
	// This broadcasts state updates directly to ALL peers (not probabilistic gossip)
	// to ensure reliable delivery of committed state changes.
	n.pbftNode.RegisterStateReplicator(func(req *consensus.Request) error {
		// Get the ticket state that was just committed
		ticket, err := n.stateMachine.GetTicket(req.TicketID)
		if err != nil {
			return fmt.Errorf("failed to get committed ticket state: %w", err)
		}

		// Convert to protobuf state
		var protoState pb.State
		switch ticket.State {
		case state.StateValidated:
			protoState = pb.State_VALIDATED
		case state.StateConsumed:
			protoState = pb.State_CONSUMED
		case state.StateDisputed:
			protoState = pb.State_DISPUTED
		default:
			protoState = pb.State_PENDING
		}

		// Convert vector clock to protobuf format for proper causality ordering
		// on receiving nodes (ensures MergeState can correctly compare versions)
		var protoVC *pb.VectorClock
		if ticket.VectorClock != nil {
			protoVC = &pb.VectorClock{
				Clocks: ticket.VectorClock.GetAll(),
			}
		}

		// Create state update message with consensus confirmation.
		// Include Data (as Metadata) and VectorClock so receiving nodes
		// get the full ticket state and can properly order concurrent updates.
		update := &pb.StateUpdate{
			Tickets: []*pb.TicketState{{
				TicketId:    ticket.ID,
				State:       protoState,
				ValidatorId: ticket.ValidatorID,
				LastUpdated: ticket.Timestamp,
				VectorClock: protoVC,
				Metadata:    ticket.Data,
			}},
			NodeId: n.nodeID,
		}

		// Serialize state update
		payload, err := proto.Marshal(update)
		if err != nil {
			return fmt.Errorf("failed to marshal state update: %w", err)
		}

		// Wrap in ValidatorMessage with STATE_UPDATE type
		msg := &pb.ValidatorMessage{
			Type:      pb.ValidatorMessage_STATE_UPDATE,
			Payload:   payload,
			SenderId:  n.nodeID,
			Timestamp: time.Now().Unix(),
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal ValidatorMessage: %w", err)
		}

		// Broadcast directly to ALL peers (reliable delivery, not probabilistic gossip)
		ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
		defer cancel()

		if err := n.p2pHost.Broadcast(ctx, data); err != nil {
			return fmt.Errorf("failed to broadcast state update: %w", err)
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

	// Register handler for state synchronization messages
	router.RegisterStateSyncHandler(func(msgType pb.ValidatorMessage_Type, payload []byte) error {
		switch msgType {
		case pb.ValidatorMessage_STATE_SYNC_REQUEST:
			return n.handleStateSyncRequest(payload)
		case pb.ValidatorMessage_STATE_SYNC_RESPONSE:
			return n.handleStateSyncResponse(payload)
		default:
			return fmt.Errorf("unknown state sync message type: %v", msgType)
		}
	})
}

// setupPeerCallbacks sets up callbacks for peer connection events
func (n *ValidatorNode) setupPeerCallbacks() {
	// Set up callback for when peers successfully connect
	n.discovery.OnPeerFound(func(peerInfo peer.AddrInfo) {
		// Check if peer is actually connected
		if n.p2pHost.Host().Network().Connectedness(peerInfo.ID) == 1 {
			fmt.Printf("✅ Peer connected: %s - requesting state synchronization\n", peerInfo.ID)
			go n.requestStateSync(peerInfo.ID)
		}
	})
}

// requestStateSync requests state synchronization from a newly connected peer
func (n *ValidatorNode) requestStateSync(peerID peer.ID) {
	// Get current sequence number from PBFT
	currentSeq := n.pbftNode.GetSequence()

	fmt.Printf("StateSync: Requesting state from peer %s (current seq: %d)\n", peerID, currentSeq)

	// Create state sync request.
	// Use libp2p peer ID (not nodeID) so the responder can SendTo us.
	request := &pb.StateSyncRequest{
		RequesterId:  n.p2pHost.ID().String(),
		LastSequence: currentSeq,
		Timestamp:    time.Now().Unix(),
	}

	// Serialize request
	payload, err := proto.Marshal(request)
	if err != nil {
		fmt.Printf("StateSync: Failed to marshal state sync request: %v\n", err)
		return
	}

	// Wrap in ValidatorMessage
	msg := &pb.ValidatorMessage{
		Type:      pb.ValidatorMessage_STATE_SYNC_REQUEST,
		Payload:   payload,
		SenderId:  n.nodeID,
		Timestamp: time.Now().Unix(),
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		fmt.Printf("StateSync: Failed to marshal ValidatorMessage: %v\n", err)
		return
	}

	// Send to specific peer
	ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
	defer cancel()

	if err := n.p2pHost.SendTo(ctx, peerID.String(), data); err != nil {
		fmt.Printf("StateSync: Failed to send state sync request to %s: %v\n", peerID, err)
	} else {
		fmt.Printf("StateSync: Sent state sync request to %s\n", peerID)
	}
}

// handleStateSyncRequest handles incoming state sync requests from peers
func (n *ValidatorNode) handleStateSyncRequest(payload []byte) error {
	var request pb.StateSyncRequest
	if err := proto.Unmarshal(payload, &request); err != nil {
		return fmt.Errorf("failed to unmarshal state sync request: %w", err)
	}

	fmt.Printf("StateSync: Received state sync request from %s (their seq: %d)\n",
		request.RequesterId, request.LastSequence)

	// Get current PBFT state
	currentSeq := n.pbftNode.GetSequence()
	currentView := n.pbftNode.GetView()

	// Get validated tickets from state machine
	validatedTickets := n.stateMachine.GetAllTickets()

	// Convert tickets to protobuf format
	pbTickets := make([]*pb.TicketState, 0, len(validatedTickets))
	consensusLog := make([]string, 0)

	for _, ticket := range validatedTickets {
		var protoState pb.State
		switch ticket.State {
		case state.StateValidated:
			protoState = pb.State_VALIDATED
			consensusLog = append(consensusLog, ticket.ID)
		case state.StateConsumed:
			protoState = pb.State_CONSUMED
		case state.StateDisputed:
			protoState = pb.State_DISPUTED
		default:
			continue // Skip non-consensus states
		}

		pbTickets = append(pbTickets, &pb.TicketState{
			TicketId:    ticket.ID,
			State:       protoState,
			ValidatorId: ticket.ValidatorID,
			LastUpdated: ticket.Timestamp,
			Metadata:    ticket.Data,
		})
	}

	// Create response
	response := &pb.StateSyncResponse{
		ResponderId:      n.nodeID,
		CurrentSequence:  currentSeq,
		CurrentView:      currentView,
		ValidatedTickets: pbTickets,
		ConsensusLog:     consensusLog,
		Timestamp:        time.Now().Unix(),
	}

	// Serialize response
	responsePayload, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal state sync response: %w", err)
	}

	// Wrap in ValidatorMessage
	msg := &pb.ValidatorMessage{
		Type:      pb.ValidatorMessage_STATE_SYNC_RESPONSE,
		Payload:   responsePayload,
		SenderId:  n.nodeID,
		Timestamp: time.Now().Unix(),
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal ValidatorMessage: %w", err)
	}

	// Send response back to requester
	ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
	defer cancel()

	if err := n.p2pHost.SendTo(ctx, request.RequesterId, data); err != nil {
		return fmt.Errorf("failed to send state sync response: %w", err)
	}

	fmt.Printf("StateSync: Sent state sync response to %s (%d tickets, seq: %d)\n",
		request.RequesterId, len(pbTickets), currentSeq)

	return nil
}

// handleStateSyncResponse handles incoming state sync responses from peers
func (n *ValidatorNode) handleStateSyncResponse(payload []byte) error {
	var response pb.StateSyncResponse
	if err := proto.Unmarshal(payload, &response); err != nil {
		return fmt.Errorf("failed to unmarshal state sync response: %w", err)
	}

	fmt.Printf("StateSync: Received state sync response from %s (seq: %d, view: %d, tickets: %d)\n",
		response.ResponderId, response.CurrentSequence, response.CurrentView, len(response.ValidatedTickets))

	currentSeq := n.pbftNode.GetSequence()

	// If peer has newer state, synchronize
	if response.CurrentSequence > currentSeq {
		fmt.Printf("StateSync: ⚠️  Peer %s has newer state (peer seq: %d, our seq: %d)\n",
			response.ResponderId, response.CurrentSequence, currentSeq)

		// Synchronize validated tickets
		syncedCount := 0
		for _, pbTicket := range response.ValidatedTickets {
			// Check if we already have this ticket
			if n.stateMachine.HasTicket(pbTicket.TicketId) {
				continue
			}

			// Only sync VALIDATED tickets (others are not consensus-approved)
			if pbTicket.State == pb.State_VALIDATED {
				fmt.Printf("StateSync: Syncing ticket %s from peer %s\n", pbTicket.TicketId, response.ResponderId)

				// Add ticket to our state (bypassing consensus since it was already agreed upon)
				if err := n.stateMachine.SyncTicket(pbTicket.TicketId, response.ResponderId, pbTicket.Metadata); err != nil {
					fmt.Printf("StateSync: Failed to sync ticket %s: %v\n", pbTicket.TicketId, err)
				} else {
					syncedCount++
					// Persist synced ticket to survive future restarts
					n.persistTicket(pbTicket.TicketId)
				}
			}
		}

		fmt.Printf("StateSync: ✅ Synchronized %d tickets from peer %s\n", syncedCount, response.ResponderId)
	} else if response.CurrentSequence == currentSeq {
		fmt.Printf("StateSync: ✅ State is synchronized with peer %s (seq: %d)\n",
			response.ResponderId, currentSeq)
	} else {
		fmt.Printf("StateSync: ℹ️  Our state is newer than peer %s (our seq: %d, peer seq: %d)\n",
			response.ResponderId, currentSeq, response.CurrentSequence)
	}

	return nil
}

// API interface implementation (for api.ValidatorInterface)

func (n *ValidatorNode) ValidateTicket(ticketID string, data []byte) error {
	// Check if ticket is already validated (early rejection to avoid consensus overhead)
	if n.stateMachine.HasTicket(ticketID) {
		ticket, err := n.stateMachine.GetTicket(ticketID)
		if err == nil && ticket != nil && ticket.State == state.StateValidated {
			return fmt.Errorf("ticket %s already validated", ticketID)
		}
	}

	// Create consensus request with unique ID for callback tracking
	requestID := fmt.Sprintf("%s-%d-%s", n.nodeID, time.Now().UnixNano(), ticketID)
	req := &consensus.Request{
		RequestID: requestID,
		TicketID:  ticketID,
		Operation: "VALIDATE",
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	// If this node is the primary, propose and wait for consensus
	if n.pbftNode.IsPrimary() {
		fmt.Printf("Node %s: ✅ Received validation request for ticket %s (I am primary)\n", n.nodeID, ticketID)

		// Check peer connectivity for multi-node setup
		peerCount := n.p2pHost.GetPeerCount()
		expectedPeers := n.totalNodes - 1 // All nodes except self

		if expectedPeers > 0 && peerCount == 0 {
			return fmt.Errorf("no peers connected, cannot reach consensus (expected %d peers)", expectedPeers)
		}
		if peerCount < expectedPeers {
			fmt.Printf("Node %s: ⚠️  WARNING - only %d/%d peers connected\n", n.nodeID, peerCount, expectedPeers)
		}

		// Create result channel for synchronous waiting
		resultChan := make(chan error, 1)

		// Register callback to be notified when consensus completes
		n.pbftNode.RegisterCallback(requestID, func(success bool, err error) {
			if success {
				resultChan <- nil
			} else {
				if err != nil {
					resultChan <- err
				} else {
					resultChan <- fmt.Errorf("consensus failed for ticket %s", ticketID)
				}
			}
		})

		// Propose the request (non-blocking, consensus runs asynchronously)
		if err := n.pbftNode.ProposeRequest(req); err != nil {
			// Proposal failed, no need to wait for callback
			return err
		}

		// Wait for consensus to complete or timeout
		// Use a timeout slightly longer than the view timeout to allow for consensus
		consensusTimeout := 20 * time.Second
		select {
		case err := <-resultChan:
			if err != nil {
				return fmt.Errorf("consensus failed: %w", err)
			}
			fmt.Printf("Node %s: ✅ Consensus completed successfully for ticket %s\n", n.nodeID, ticketID)
			return nil
		case <-time.After(consensusTimeout):
			return fmt.Errorf("consensus timeout: validation for ticket %s did not complete within %v", ticketID, consensusTimeout)
		case <-n.ctx.Done():
			return fmt.Errorf("node shutting down")
		}
	}

	// Non-primary nodes forward the request to all peers (primary will handle it)
	primary := n.pbftNode.GetPrimary()
	fmt.Printf("Node %s: 📤 Forwarding validation request for ticket %s to primary %s\n", n.nodeID, ticketID, primary)

	// Wait for peers to connect (with retry)
	// This handles the race condition where API starts before peer discovery completes
	maxRetries := 5
	retryDelay := 1 * time.Second
	var peerCount int

	for i := 0; i < maxRetries; i++ {
		peerCount = n.p2pHost.GetPeerCount()
		if peerCount > 0 {
			break
		}

		if i == 0 {
			fmt.Printf("Node %s: ⏳ Waiting for peer connections (attempt %d/%d)...\n", n.nodeID, i+1, maxRetries)
		} else {
			fmt.Printf("Node %s: ⏳ Retrying... (attempt %d/%d)\n", n.nodeID, i+1, maxRetries)
		}

		select {
		case <-time.After(retryDelay):
			continue
		case <-n.ctx.Done():
			return fmt.Errorf("node shutting down")
		}
	}

	if peerCount == 0 {
		return fmt.Errorf("no peers connected after %d attempts, cannot forward request to primary %s. Please wait for peer discovery to complete", maxRetries, primary)
	}

	fmt.Printf("Node %s: ✅ Connected to %d peer(s), forwarding request\n", n.nodeID, peerCount)

	// Serialize the request
	payload, err := consensus.SerializeRequest(req)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	// Wrap in ValidatorMessage with CLIENT_REQUEST type
	msgData, err := consensus.SerializePBFTMessage(pb.ValidatorMessage_CLIENT_REQUEST, payload, n.nodeID)
	if err != nil {
		return fmt.Errorf("failed to wrap client request: %w", err)
	}

	// Broadcast to all peers (primary will pick it up)
	broadcastCtx, broadcastCancel := context.WithTimeout(n.ctx, 3*time.Second)
	if err := n.p2pHost.Broadcast(broadcastCtx, msgData); err != nil {
		broadcastCancel()
		return fmt.Errorf("failed to forward request to primary: %w", err)
	}
	broadcastCancel()

	fmt.Printf("Node %s: 📤 Successfully forwarded request for ticket %s to %d peers, waiting for consensus...\n", n.nodeID, ticketID, peerCount)

	// Wait for consensus to replicate the ticket to this node.
	// The primary will process the forwarded request, run PBFT consensus,
	// and the StateReplicator will broadcast the committed state to all peers
	// (including this node). Poll local state until the ticket appears.
	// If the primary rejects due to insufficient peers or the broadcast is lost,
	// retry the broadcast every 5 seconds.
	consensusTimeout := 20 * time.Second
	deadline := time.After(consensusTimeout)
	pollInterval := 200 * time.Millisecond
	retryInterval := 5 * time.Second
	retryTicker := time.NewTicker(retryInterval)
	defer retryTicker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("consensus timeout: validation for ticket %s did not complete within %v", ticketID, consensusTimeout)
		case <-n.ctx.Done():
			return fmt.Errorf("node shutting down")
		case <-retryTicker.C:
			// Re-broadcast in case primary rejected or message was lost
			retryCtx, retryCancel := context.WithTimeout(n.ctx, 3*time.Second)
			if err := n.p2pHost.Broadcast(retryCtx, msgData); err == nil {
				fmt.Printf("Node %s: 🔄 Re-broadcast CLIENT_REQUEST for ticket %s\n", n.nodeID, ticketID)
			}
			retryCancel()
		case <-time.After(pollInterval):
			ticket, err := n.stateMachine.GetTicket(ticketID)
			if err == nil && ticket != nil && ticket.State == state.StateValidated {
				fmt.Printf("Node %s: ✅ Ticket %s confirmed validated via consensus replication\n", n.nodeID, ticketID)
				return nil
			}
		}
	}
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

func (n *ValidatorNode) GetPeers() ([]api.PeerInfo, error) {
	// Get connected peers from P2P host
	peers := n.p2pHost.GetConnectedPeers()

	// Convert to API format
	peerInfos := make([]api.PeerInfo, 0, len(peers))
	for _, p := range peers {
		addrs := make([]string, len(p.Addrs))
		for i, addr := range p.Addrs {
			addrs[i] = addr.String()
		}

		peerInfos = append(peerInfos, api.PeerInfo{
			ID:    p.ID.String(),
			Addrs: addrs,
		})
	}

	return peerInfos, nil
}

func (n *ValidatorNode) GetConfig() (interface{}, error) {
	return map[string]interface{}{
		"node_id":      n.nodeID,
		"total_nodes":  n.totalNodes,
		"is_primary":   n.pbftNode.IsPrimary(),
		"current_view": n.pbftNode.GetView(),
	}, nil
}

// persistTicket saves a ticket's current state to BoltDB for crash recovery.
func (n *ValidatorNode) persistTicket(ticketID string) {
	ticket, err := n.stateMachine.GetTicket(ticketID)
	if err != nil {
		fmt.Printf("Node %s: ⚠️  Failed to get ticket %s for persistence: %v\n", n.nodeID, ticketID, err)
		return
	}

	var vc map[string]uint64
	if ticket.VectorClock != nil {
		vc = ticket.VectorClock.GetAll()
	}

	record := &storage.TicketRecord{
		ID:          ticket.ID,
		State:       string(ticket.State),
		Data:        ticket.Data,
		ValidatorID: ticket.ValidatorID,
		Timestamp:   ticket.Timestamp,
		VectorClock: vc,
		Metadata:    ticket.Metadata,
	}

	if err := n.storage.SaveTicket(record); err != nil {
		fmt.Printf("Node %s: ⚠️  Failed to persist ticket %s: %v\n", n.nodeID, ticketID, err)
	} else {
		fmt.Printf("Node %s: 💾 Persisted ticket %s (state: %s)\n", n.nodeID, ticketID, ticket.State)
	}
}

// loadTicketsFromStore loads all persisted tickets into the state machine on startup.
func (n *ValidatorNode) loadTicketsFromStore() {
	records, err := n.storage.GetAllTickets()
	if err != nil {
		fmt.Printf("Node %s: ⚠️  Failed to load tickets from store: %v\n", n.nodeID, err)
		return
	}

	if len(records) == 0 {
		return
	}

	tickets := make([]*state.Ticket, 0, len(records))
	for _, rec := range records {
		vc := state.NewVectorClock(n.nodeID)
		for k, v := range rec.VectorClock {
			vc.Update(k, v)
		}

		tickets = append(tickets, &state.Ticket{
			ID:          rec.ID,
			State:       state.TicketState(rec.State),
			Data:        rec.Data,
			ValidatorID: rec.ValidatorID,
			Timestamp:   rec.Timestamp,
			VectorClock: vc,
			Metadata:    rec.Metadata,
		})
	}

	if err := n.stateMachine.MergeState(tickets); err != nil {
		fmt.Printf("Node %s: ⚠️  Failed to merge stored tickets: %v\n", n.nodeID, err)
		return
	}

	fmt.Printf("Node %s: ✅ Loaded %d tickets from persistent store\n", n.nodeID, len(records))
}
