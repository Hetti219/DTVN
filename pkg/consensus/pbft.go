package consensus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// State represents the PBFT state
type State string

const (
	StateIdle       State = "IDLE"
	StatePrePrepare State = "PRE_PREPARE"
	StatePrepare    State = "PREPARE"
	StateCommit     State = "COMMIT"
	StateCommitted  State = "COMMITTED"
)

// PBFTNode represents a PBFT consensus node
type PBFTNode struct {
	nodeID       string
	view         int64
	sequence     int64
	state        State
	f            int // Maximum number of Byzantine nodes tolerated
	totalNodes   int
	isPrimary    bool
	prepareLog   map[int64]map[string]*PrepareMsg
	commitLog    map[int64]map[string]*CommitMsg
	requestLog   map[int64]*Request
	messageQueue chan ConsensusMessage
	viewTimer    *time.Timer
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
	handlers     []ConsensusHandler
	broadcaster  MessageBroadcaster
	checkpoints  map[int64]*Checkpoint
}

// PrepareMsg represents a PREPARE message
type PrepareMsg struct {
	View     int64
	Sequence int64
	Digest   string
	NodeID   string
}

// GetView returns the view number
func (m *PrepareMsg) GetView() int64 {
	return m.View
}

// GetSequence returns the sequence number
func (m *PrepareMsg) GetSequence() int64 {
	return m.Sequence
}

// CommitMsg represents a COMMIT message
type CommitMsg struct {
	View     int64
	Sequence int64
	Digest   string
	NodeID   string
}

// GetView returns the view number
func (m *CommitMsg) GetView() int64 {
	return m.View
}

// GetSequence returns the sequence number
func (m *CommitMsg) GetSequence() int64 {
	return m.Sequence
}

// PrePrepareMsg represents a PRE-PREPARE message
type PrePrepareMsg struct {
	View     int64
	Sequence int64
	Digest   string
	Request  *Request
}

// GetView returns the view number
func (m *PrePrepareMsg) GetView() int64 {
	return m.View
}

// GetSequence returns the sequence number
func (m *PrePrepareMsg) GetSequence() int64 {
	return m.Sequence
}

// Request represents a client request
type Request struct {
	RequestID string
	TicketID  string
	Operation string
	Data      []byte
	Timestamp int64
	ClientSig []byte
}

// Checkpoint represents a state checkpoint
type Checkpoint struct {
	Sequence    int64
	StateDigest string
	NodeID      string
	Timestamp   int64
}

// ConsensusMessage is the interface for all consensus messages
type ConsensusMessage interface {
	GetView() int64
	GetSequence() int64
}

// ConsensusHandler is called when consensus is reached
type ConsensusHandler func(*Request) error

// MessageBroadcaster interface for broadcasting messages
type MessageBroadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	SendTo(ctx context.Context, nodeID string, data []byte) error
}

// Config holds PBFT configuration
type Config struct {
	NodeID      string
	TotalNodes  int
	IsPrimary   bool
	ViewTimeout time.Duration
}

// NewPBFTNode creates a new PBFT node
func NewPBFTNode(ctx context.Context, cfg *Config, broadcaster MessageBroadcaster) (*PBFTNode, error) {
	// Calculate f (max Byzantine nodes: f = (n-1)/3)
	f := (cfg.TotalNodes - 1) / 3

	nodeCtx, cancel := context.WithCancel(ctx)

	viewTimeout := cfg.ViewTimeout
	if viewTimeout == 0 {
		viewTimeout = 5 * time.Second
	}

	node := &PBFTNode{
		nodeID:       cfg.NodeID,
		view:         0,
		sequence:     0,
		state:        StateIdle,
		f:            f,
		totalNodes:   cfg.TotalNodes,
		isPrimary:    cfg.IsPrimary,
		prepareLog:   make(map[int64]map[string]*PrepareMsg),
		commitLog:    make(map[int64]map[string]*CommitMsg),
		requestLog:   make(map[int64]*Request),
		messageQueue: make(chan ConsensusMessage, 1000),
		viewTimer:    time.NewTimer(viewTimeout),
		ctx:          nodeCtx,
		cancel:       cancel,
		handlers:     make([]ConsensusHandler, 0),
		broadcaster:  broadcaster,
		checkpoints:  make(map[int64]*Checkpoint),
	}

	return node, nil
}

// Start begins the PBFT consensus process
func (n *PBFTNode) Start() {
	go n.processMessages()
	go n.monitorViewTimeout()
}

// ProposeRequest proposes a new request (only primary can call this)
func (n *PBFTNode) ProposeRequest(req *Request) error {
	n.mu.Lock()

	if !n.isPrimary {
		n.mu.Unlock()
		return fmt.Errorf("only primary can propose requests")
	}

	fmt.Printf("PBFT: Node %s proposing request for ticket %s (op: %s)\n", n.nodeID, req.TicketID, req.Operation)

	// Increment sequence number
	n.sequence++
	seq := n.sequence

	// Store request
	n.requestLog[seq] = req

	// Create digest
	digest := n.computeDigest(req)

	fmt.Printf("PBFT: Created PRE-PREPARE for seq %d, view %d\n", seq, n.view)

	// Create PRE-PREPARE message
	prePrepare := &PrePrepareMsg{
		View:     n.view,
		Sequence: seq,
		Digest:   digest,
		Request:  req,
	}

	// Unlock before serialization and broadcasting
	n.mu.Unlock()

	// Broadcast PRE-PREPARE using protobuf
	payload, err := SerializePrePrepare(prePrepare)
	if err != nil {
		fmt.Printf("Failed to serialize PRE-PREPARE: %v\n", err)
		return err
	}
	data, err := SerializePBFTMessage(0, payload, n.nodeID) // 0 = PRE_PREPARE type
	if err != nil {
		fmt.Printf("Failed to wrap PRE-PREPARE message: %v\n", err)
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
		defer cancel()
		if err := n.broadcaster.Broadcast(ctx, data); err != nil {
			fmt.Printf("Failed to broadcast PRE-PREPARE: %v\n", err)
		}
	}()

	// Process locally (handlePrePrepare will acquire its own lock)
	return n.handlePrePrepare(prePrepare)
}

// HandlePrePrepare handles incoming PRE-PREPARE messages
func (n *PBFTNode) handlePrePrepare(msg *PrePrepareMsg) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Validate view
	if msg.View != n.view {
		return fmt.Errorf("view mismatch: expected %d, got %d", n.view, msg.View)
	}

	// Validate digest
	digest := n.computeDigest(msg.Request)
	if digest != msg.Digest {
		return fmt.Errorf("digest mismatch")
	}

	// Store request
	n.requestLog[msg.Sequence] = msg.Request

	// Update state
	n.state = StatePrepare

	// Send PREPARE message
	prepare := &PrepareMsg{
		View:     msg.View,
		Sequence: msg.Sequence,
		Digest:   msg.Digest,
		NodeID:   n.nodeID,
	}

	// Initialize prepare log for this sequence
	if n.prepareLog[msg.Sequence] == nil {
		n.prepareLog[msg.Sequence] = make(map[string]*PrepareMsg)
	}
	n.prepareLog[msg.Sequence][n.nodeID] = prepare

	// Broadcast PREPARE using protobuf
	payload, err := SerializePrepare(prepare)
	if err != nil {
		fmt.Printf("Failed to serialize PREPARE: %v\n", err)
		return err
	}
	data, err := SerializePBFTMessage(1, payload, n.nodeID) // 1 = PREPARE type
	if err != nil {
		fmt.Printf("Failed to wrap PREPARE message: %v\n", err)
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
		defer cancel()
		if err := n.broadcaster.Broadcast(ctx, data); err != nil {
			fmt.Printf("Failed to broadcast PREPARE: %v\n", err)
		}
	}()

	// Check if we already have quorum (important for single-node case)
	if n.checkPrepareQuorum(msg.Sequence) {
		return n.moveToCommitPhase(msg.Sequence, msg.Digest)
	}

	return nil
}

// HandlePrepare handles incoming PREPARE messages
func (n *PBFTNode) HandlePrepare(msg *PrepareMsg) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Validate view
	if msg.View != n.view {
		return fmt.Errorf("view mismatch")
	}

	// Store PREPARE message
	if n.prepareLog[msg.Sequence] == nil {
		n.prepareLog[msg.Sequence] = make(map[string]*PrepareMsg)
	}
	n.prepareLog[msg.Sequence][msg.NodeID] = msg

	// Check if we have quorum (2f+1 PREPARE messages)
	if n.checkPrepareQuorum(msg.Sequence) {
		return n.moveToCommitPhase(msg.Sequence, msg.Digest)
	}

	return nil
}

// HandleCommit handles incoming COMMIT messages
func (n *PBFTNode) HandleCommit(msg *CommitMsg) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Validate view
	if msg.View != n.view {
		return fmt.Errorf("view mismatch")
	}

	// Store COMMIT message
	if n.commitLog[msg.Sequence] == nil {
		n.commitLog[msg.Sequence] = make(map[string]*CommitMsg)
	}
	n.commitLog[msg.Sequence][msg.NodeID] = msg

	// Check if we have quorum (2f+1 COMMIT messages)
	if n.checkCommitQuorum(msg.Sequence) {
		return n.executeRequest(msg.Sequence)
	}

	return nil
}

// checkPrepareQuorum checks if we have enough PREPARE messages
func (n *PBFTNode) checkPrepareQuorum(sequence int64) bool {
	prepares := n.prepareLog[sequence]
	required := 2*n.f + 1
	// Ensure at least 1 is required even for single node
	if required < 1 {
		required = 1
	}
	hasQuorum := len(prepares) >= required
	if hasQuorum {
		fmt.Printf("PBFT: Prepare quorum reached for seq %d (%d/%d messages)\n", sequence, len(prepares), required)
	}
	return hasQuorum
}

// checkCommitQuorum checks if we have enough COMMIT messages
func (n *PBFTNode) checkCommitQuorum(sequence int64) bool {
	commits := n.commitLog[sequence]
	required := 2*n.f + 1
	// Ensure at least 1 is required even for single node
	if required < 1 {
		required = 1
	}
	hasQuorum := len(commits) >= required
	if hasQuorum {
		fmt.Printf("PBFT: Commit quorum reached for seq %d (%d/%d messages)\n", sequence, len(commits), required)
	}
	return hasQuorum
}

// moveToCommitPhase moves to the commit phase
func (n *PBFTNode) moveToCommitPhase(sequence int64, digest string) error {
	n.state = StateCommit

	// Send COMMIT message
	commit := &CommitMsg{
		View:     n.view,
		Sequence: sequence,
		Digest:   digest,
		NodeID:   n.nodeID,
	}

	// Initialize commit log for this sequence
	if n.commitLog[sequence] == nil {
		n.commitLog[sequence] = make(map[string]*CommitMsg)
	}
	n.commitLog[sequence][n.nodeID] = commit

	// Broadcast COMMIT using protobuf
	payload, err := SerializeCommit(commit)
	if err != nil {
		fmt.Printf("Failed to serialize COMMIT: %v\n", err)
		return err
	}
	data, err := SerializePBFTMessage(2, payload, n.nodeID) // 2 = COMMIT type
	if err != nil {
		fmt.Printf("Failed to wrap COMMIT message: %v\n", err)
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
		defer cancel()
		if err := n.broadcaster.Broadcast(ctx, data); err != nil {
			fmt.Printf("Failed to broadcast COMMIT: %v\n", err)
		}
	}()

	// Check if we already have quorum (important for single-node case)
	if n.checkCommitQuorum(sequence) {
		return n.executeRequest(sequence)
	}

	return nil
}

// executeRequest executes a request after consensus is reached
func (n *PBFTNode) executeRequest(sequence int64) error {
	req, exists := n.requestLog[sequence]
	if !exists {
		return fmt.Errorf("request not found for sequence %d", sequence)
	}

	fmt.Printf("PBFT: ✅ Consensus reached for seq %d, executing request for ticket %s\n", sequence, req.TicketID)

	n.state = StateCommitted

	// Call registered handlers
	for _, handler := range n.handlers {
		if err := handler(req); err != nil {
			fmt.Printf("PBFT: Handler error: %v\n", err)
			return err
		}
	}

	fmt.Printf("PBFT: ✅ Request executed successfully for ticket %s\n", req.TicketID)

	// Reset to idle
	n.state = StateIdle

	// Reset view timer
	n.viewTimer.Reset(5 * time.Second)

	return nil
}

// RegisterHandler registers a consensus handler
func (n *PBFTNode) RegisterHandler(handler ConsensusHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers = append(n.handlers, handler)
}

// processMessages processes incoming consensus messages
func (n *PBFTNode) processMessages() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case msg := <-n.messageQueue:
			// Process based on message type
			switch m := msg.(type) {
			case *PrePrepareMsg:
				if err := n.handlePrePrepare(m); err != nil {
					fmt.Printf("Error handling PRE-PREPARE: %v\n", err)
				}
			case *PrepareMsg:
				if err := n.HandlePrepare(m); err != nil {
					fmt.Printf("Error handling PREPARE: %v\n", err)
				}
			case *CommitMsg:
				if err := n.HandleCommit(m); err != nil {
					fmt.Printf("Error handling COMMIT: %v\n", err)
				}
			default:
				fmt.Printf("Unknown message type: %T\n", m)
			}
		}
	}
}

// monitorViewTimeout monitors for view timeout and initiates view change
func (n *PBFTNode) monitorViewTimeout() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.viewTimer.C:
			n.initiateViewChange()
		}
	}
}

// initiateViewChange initiates a view change
func (n *PBFTNode) initiateViewChange() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Only trigger view change if there's a pending consensus operation
	// If the node is idle, just reset the timer and continue
	if n.state == StateIdle {
		n.viewTimer.Reset(5 * time.Second)
		return
	}

	// There's a pending request that hasn't completed - trigger view change
	fmt.Printf("Node %s: View timeout while in state %s, initiating view change\n", n.nodeID, n.state)

	n.view++

	// Calculate which node should be primary for this view
	primaryIndex := n.view % int64(n.totalNodes)
	expectedPrimaryID := fmt.Sprintf("node%d", primaryIndex)

	// Update isPrimary based on whether this node matches the expected primary
	n.isPrimary = (n.nodeID == expectedPrimaryID)

	fmt.Printf("Node %s: View change to view %d, primary should be %s, isPrimary: %v\n",
		n.nodeID, n.view, expectedPrimaryID, n.isPrimary)

	// Reset state
	n.state = StateIdle
	n.viewTimer.Reset(5 * time.Second)
}

// GetView returns the current view
func (n *PBFTNode) GetView() int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.view
}

// GetSequence returns the current sequence number
func (n *PBFTNode) GetSequence() int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.sequence
}

// IsPrimary returns whether this node is the primary
func (n *PBFTNode) IsPrimary() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isPrimary
}

// GetPrimary returns the node ID of the current primary
func (n *PBFTNode) GetPrimary() string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Primary is determined by view number mod total nodes
	primaryIndex := n.view % int64(n.totalNodes)

	// Map index to node ID (simplified: assume node0, node1, etc.)
	return fmt.Sprintf("node%d", primaryIndex)
}

// HandleNetworkMessage processes incoming PBFT messages from the network layer
func (n *PBFTNode) HandleNetworkMessage(msgType int32, payload []byte) error {
	var msg ConsensusMessage
	var err error

	// Deserialize based on message type
	switch msgType {
	case 0: // PRE_PREPARE
		msg, err = DeserializePrePrepare(payload)
	case 1: // PREPARE
		msg, err = DeserializePrepare(payload)
	case 2: // COMMIT
		msg, err = DeserializeCommit(payload)
	case 7: // CLIENT_REQUEST - forwarded from non-primary node
		// Deserialize the request
		req, err := DeserializeRequest(payload)
		if err != nil {
			return fmt.Errorf("failed to deserialize client request: %w", err)
		}

		// Only primary should receive and process forwarded client requests
		if !n.IsPrimary() {
			return fmt.Errorf("received client request but not primary")
		}

		fmt.Printf("PBFT: Primary %s received forwarded client request for ticket %s\n", n.nodeID, req.TicketID)

		// Propose the request through consensus
		return n.ProposeRequest(req)
	default:
		return fmt.Errorf("unsupported PBFT message type: %d", msgType)
	}

	if err != nil {
		return fmt.Errorf("failed to deserialize message: %w", err)
	}

	// Inject into message queue (this fixes the broken message flow)
	select {
	case n.messageQueue <- msg:
		return nil
	case <-n.ctx.Done():
		return n.ctx.Err()
	default:
		return fmt.Errorf("message queue full, dropping message")
	}
}

// Close shuts down the PBFT node
func (n *PBFTNode) Close() error {
	n.cancel()
	return nil
}

// Helper functions

// computeDigest computes the digest of a request
func (n *PBFTNode) computeDigest(req *Request) string {
	data := fmt.Sprintf("%s:%s:%s:%d", req.RequestID, req.TicketID, req.Operation, req.Timestamp)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// Old string serialization functions removed - now using protobuf
// See messages.go for the new SerializePrePrepare/Prepare/Commit functions
