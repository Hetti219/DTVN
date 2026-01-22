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

	"github.com/Hetti219/distributed-ticket-validation/pkg/api"
	"github.com/Hetti219/distributed-ticket-validation/pkg/consensus"
	"github.com/Hetti219/distributed-ticket-validation/pkg/gossip"
	"github.com/Hetti219/distributed-ticket-validation/pkg/network"
	"github.com/Hetti219/distributed-ticket-validation/pkg/state"
	"github.com/Hetti219/distributed-ticket-validation/pkg/storage"
	pb "github.com/Hetti219/distributed-ticket-validation/proto"
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
		return n.stateMachine.MergeState(tickets)
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

	// Create state sync request
	request := &pb.StateSyncRequest{
		RequesterId:  n.nodeID,
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
		})
	}

	// Create response
	response := &pb.StateSyncResponse{
		ResponderId:       n.nodeID,
		CurrentSequence:   currentSeq,
		CurrentView:       currentView,
		ValidatedTickets:  pbTickets,
		ConsensusLog:      consensusLog,
		Timestamp:         time.Now().Unix(),
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
				if err := n.stateMachine.SyncTicket(pbTicket.TicketId, response.ResponderId, nil); err != nil {
					fmt.Printf("StateSync: Failed to sync ticket %s: %v\n", pbTicket.TicketId, err)
				} else {
					syncedCount++
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

	// Create consensus request
	req := &consensus.Request{
		RequestID: fmt.Sprintf("%s-%d", n.nodeID, time.Now().Unix()),
		TicketID:  ticketID,
		Operation: "VALIDATE",
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	// If this node is the primary, propose directly
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

		return n.pbftNode.ProposeRequest(req)
	}

	// Non-primary nodes forward the request to all peers (primary will handle it)
	primary := n.pbftNode.GetPrimary()
	fmt.Printf("Node %s: 📤 Forwarding validation request for ticket %s to primary %s\n", n.nodeID, ticketID, primary)

	// Check if we have any peers to forward to
	peerCount := n.p2pHost.GetPeerCount()
	if peerCount == 0 {
		return fmt.Errorf("no peers connected, cannot forward request to primary %s", primary)
	}

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
	ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
	defer cancel()

	if err := n.p2pHost.Broadcast(ctx, msgData); err != nil {
		return fmt.Errorf("failed to forward request to primary: %w", err)
	}

	fmt.Printf("Node %s: ✅ Successfully forwarded request for ticket %s to %d peers\n", n.nodeID, ticketID, peerCount)
	return nil
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
