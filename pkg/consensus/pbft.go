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
	viewTimeout  time.Duration // Configurable view timeout for consensus
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
	handlers     []ConsensusHandler
	broadcaster  MessageBroadcaster
	checkpoints  map[int64]*Checkpoint

	// Callback mechanism for synchronous consensus waiting
	// Maps request ID to callback function
	requestCallbacks map[string]ConsensusCallback
	callbackMu       sync.Mutex

	// State replicator - called after consensus commits to ensure reliable state propagation
	// This provides reliable delivery beyond probabilistic gossip
	stateReplicator StateReplicator
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

// ConsensusCallback is called when consensus completes or fails for a specific request
// This enables synchronous API responses that wait for consensus
type ConsensusCallback func(success bool, err error)

// StateReplicator is called after consensus commits to ensure reliable state propagation
// to all peers. This provides guaranteed delivery beyond probabilistic gossip.
// The replicator receives the request that was committed and should broadcast the
// resulting state to all connected peers.
type StateReplicator func(req *Request) error

// MessageBroadcaster interface for broadcasting messages
type MessageBroadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	SendTo(ctx context.Context, nodeID string, data []byte) error
	GetPeerCount() int
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
		// Default to 15 seconds to allow sufficient time for PBFT three-phase commit
		// across network latency. 5 seconds was too aggressive and caused premature
		// view changes before PREPARE votes could be collected.
		viewTimeout = 15 * time.Second
	}

	node := &PBFTNode{
		nodeID:           cfg.NodeID,
		view:             0,
		sequence:         0,
		state:            StateIdle,
		f:                f,
		totalNodes:       cfg.TotalNodes,
		isPrimary:        cfg.IsPrimary,
		prepareLog:       make(map[int64]map[string]*PrepareMsg),
		commitLog:        make(map[int64]map[string]*CommitMsg),
		requestLog:       make(map[int64]*Request),
		messageQueue:     make(chan ConsensusMessage, 1000),
		viewTimer:        time.NewTimer(viewTimeout),
		viewTimeout:      viewTimeout, // Store for consistent timer resets
		ctx:              nodeCtx,
		cancel:           cancel,
		handlers:         make([]ConsensusHandler, 0),
		broadcaster:      broadcaster,
		checkpoints:      make(map[int64]*Checkpoint),
		requestCallbacks: make(map[string]ConsensusCallback),
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

	// Verify we can reach quorum with connected peers
	currentPeerCount := n.broadcaster.GetPeerCount()
	// Total active nodes = connected peers + self
	activeNodes := currentPeerCount + 1
	// Quorum requirement: 2f + 1 nodes must respond
	requiredQuorum := 2*n.f + 1

	// Special case: single node setup (totalNodes = 1)
	if n.totalNodes == 1 {
		// Single node can always reach quorum (itself)
		fmt.Printf("PBFT: Node %s proposing request for ticket %s (op: %s) [single-node mode]\n", n.nodeID, req.TicketID, req.Operation)
	} else {
		// Multi-node setup: check if we have enough peers
		if activeNodes < requiredQuorum {
			n.mu.Unlock()
			return fmt.Errorf("insufficient peers for consensus: have %d nodes (including self), need %d for quorum (connected peers: %d, expected: %d)",
				activeNodes, requiredQuorum, currentPeerCount, n.totalNodes-1)
		}

		// Warn if we don't have all expected peers
		expectedPeers := n.totalNodes - 1
		if currentPeerCount < expectedPeers {
			fmt.Printf("PBFT: ⚠️  WARNING - Node %s proposing with only %d/%d peers connected (can reach quorum but not all nodes present)\n",
				n.nodeID, currentPeerCount, expectedPeers)
		}

		fmt.Printf("PBFT: Node %s proposing request for ticket %s (op: %s) [active nodes: %d/%d, quorum: %d]\n",
			n.nodeID, req.TicketID, req.Operation, activeNodes, n.totalNodes, requiredQuorum)
	}

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

	fmt.Printf("PBFT: Node %s broadcasting PRE-PREPARE for seq %d\n", n.nodeID, seq)
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

	fmt.Printf("PBFT: Node %s received PRE-PREPARE for seq %d, view %d, ticket %s\n",
		n.nodeID, msg.Sequence, msg.View, msg.Request.TicketID)

	// Validate view
	if msg.View != n.view {
		return fmt.Errorf("view mismatch: expected %d, got %d", n.view, msg.View)
	}

	// Validate digest
	digest := n.computeDigest(msg.Request)
	if digest != msg.Digest {
		return fmt.Errorf("digest mismatch")
	}

	// Store request - this makes the request available for consensus execution.
	// PREPAREs or COMMITs may have arrived before this PRE-PREPARE due to
	// network reordering; they are stored but not acted upon until now.
	n.requestLog[msg.Sequence] = msg.Request

	// Reset view timer for this new consensus round. Without this, a node
	// that has been idle may have a nearly-expired timer, causing a premature
	// view change before consensus can complete.
	n.viewTimer.Reset(n.viewTimeout)

	// Update state
	n.state = StatePrepare

	fmt.Printf("PBFT: Node %s moving to PREPARE phase for seq %d\n", n.nodeID, msg.Sequence)

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

	fmt.Printf("PBFT: Node %s broadcasting PREPARE for seq %d\n", n.nodeID, msg.Sequence)
	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
		defer cancel()
		if err := n.broadcaster.Broadcast(ctx, data); err != nil {
			fmt.Printf("Failed to broadcast PREPARE: %v\n", err)
		}
	}()

	// Check if we already have prepare quorum (important for single-node case
	// and for catching PREPAREs that arrived before this PRE-PREPARE)
	if n.checkPrepareQuorum(msg.Sequence) {
		return n.moveToCommitPhase(msg.Sequence, msg.Digest)
	}

	// Also check if COMMITs arrived early (before PRE-PREPARE) and we already
	// have commit quorum. This handles extreme reordering where the entire
	// PREPARE+COMMIT exchange completed before PRE-PREPARE was processed.
	if n.checkCommitQuorum(msg.Sequence) {
		return n.executeRequest(msg.Sequence)
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

	// Calculate quorum requirement
	required := 2*n.f + 1
	if required < 1 {
		required = 1
	}
	current := len(n.prepareLog[msg.Sequence])

	fmt.Printf("PBFT: Node %s received PREPARE from %s for seq %d (progress: %d/%d)\n",
		n.nodeID, msg.NodeID, msg.Sequence, current, required)

	// Check if we have quorum (2f+1 PREPARE messages)
	if n.checkPrepareQuorum(msg.Sequence) {
		// Only advance to COMMIT phase if PRE-PREPARE has been processed
		// (request exists in requestLog). If PREPAREs arrive before PRE-PREPARE
		// due to network reordering, we store them and the quorum check will
		// be re-triggered when handlePrePrepare runs.
		if _, hasRequest := n.requestLog[msg.Sequence]; hasRequest {
			return n.moveToCommitPhase(msg.Sequence, msg.Digest)
		}
		fmt.Printf("PBFT: Node %s has PREPARE quorum for seq %d but waiting for PRE-PREPARE (request not yet available)\n",
			n.nodeID, msg.Sequence)
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

	// Calculate quorum requirement
	required := 2*n.f + 1
	if required < 1 {
		required = 1
	}
	current := len(n.commitLog[msg.Sequence])

	fmt.Printf("PBFT: Node %s received COMMIT from %s for seq %d (progress: %d/%d)\n",
		n.nodeID, msg.NodeID, msg.Sequence, current, required)

	// Check if we have quorum (2f+1 COMMIT messages)
	if n.checkCommitQuorum(msg.Sequence) {
		// Only execute if PRE-PREPARE has been processed (request exists).
		// If COMMITs arrive before PRE-PREPARE due to network reordering,
		// we store them and executeRequest will be called from
		// handlePrePrepare → moveToCommitPhase once the request is available.
		if _, hasRequest := n.requestLog[msg.Sequence]; hasRequest {
			return n.executeRequest(msg.Sequence)
		}
		fmt.Printf("PBFT: Node %s has COMMIT quorum for seq %d but waiting for PRE-PREPARE (request not yet available)\n",
			n.nodeID, msg.Sequence)
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

	fmt.Printf("PBFT: Node %s moving to COMMIT phase for seq %d\n", n.nodeID, sequence)

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

	fmt.Printf("PBFT: Node %s broadcasting COMMIT for seq %d\n", n.nodeID, sequence)
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
		err := fmt.Errorf("request not found for sequence %d", sequence)
		return err
	}

	fmt.Printf("PBFT: ✅ Consensus reached for seq %d, executing request for ticket %s\n", sequence, req.TicketID)

	n.state = StateCommitted

	// Call registered handlers
	var handlerErr error
	for _, handler := range n.handlers {
		if err := handler(req); err != nil {
			fmt.Printf("PBFT: Handler error: %v\n", err)
			handlerErr = err
			break
		}
	}

	// Invoke callback for this request (notifies waiting API call)
	if handlerErr != nil {
		n.invokeCallback(req.RequestID, false, handlerErr)
		return handlerErr
	}

	fmt.Printf("PBFT: ✅ Request executed successfully for ticket %s\n", req.TicketID)

	// Call state replicator to ensure reliable state propagation to all peers
	// This provides guaranteed delivery beyond probabilistic gossip by directly
	// broadcasting state updates to all connected peers after consensus commits.
	if n.stateReplicator != nil {
		if err := n.stateReplicator(req); err != nil {
			// Log the error but don't fail - state was applied locally and gossip
			// will eventually propagate it via anti-entropy
			fmt.Printf("PBFT: Warning - State replication failed (will retry via anti-entropy): %v\n", err)
		} else {
			fmt.Printf("PBFT: ✅ State replicated to all peers for ticket %s\n", req.TicketID)
		}
	}

	// Invoke callback with success
	n.invokeCallback(req.RequestID, true, nil)

	// Reset to idle
	n.state = StateIdle

	// Reset view timer
	n.viewTimer.Reset(n.viewTimeout)

	return nil
}

// RegisterHandler registers a consensus handler
func (n *PBFTNode) RegisterHandler(handler ConsensusHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers = append(n.handlers, handler)
}

// RegisterCallback registers a callback for a specific request that will be called
// when consensus completes (success or failure). This enables synchronous API responses.
func (n *PBFTNode) RegisterCallback(requestID string, callback ConsensusCallback) {
	n.callbackMu.Lock()
	defer n.callbackMu.Unlock()
	n.requestCallbacks[requestID] = callback
}

// RegisterStateReplicator registers a callback that is invoked after consensus commits
// to ensure reliable state propagation to all peers. This provides guaranteed delivery
// beyond probabilistic gossip by directly broadcasting state updates to all connected peers.
func (n *PBFTNode) RegisterStateReplicator(replicator StateReplicator) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stateReplicator = replicator
}

// unregisterCallback removes a callback for a request
func (n *PBFTNode) unregisterCallback(requestID string) {
	n.callbackMu.Lock()
	defer n.callbackMu.Unlock()
	delete(n.requestCallbacks, requestID)
}

// invokeCallback calls and removes the callback for a request
func (n *PBFTNode) invokeCallback(requestID string, success bool, err error) {
	n.callbackMu.Lock()
	callback, exists := n.requestCallbacks[requestID]
	if exists {
		delete(n.requestCallbacks, requestID)
	}
	n.callbackMu.Unlock()

	if exists && callback != nil {
		// Call the callback asynchronously to avoid blocking consensus
		go callback(success, err)
	}
}

// failPendingCallbacks fails all pending callbacks (e.g., on view change)
func (n *PBFTNode) failPendingCallbacks(reason string) {
	n.callbackMu.Lock()
	callbacks := make(map[string]ConsensusCallback)
	for k, v := range n.requestCallbacks {
		callbacks[k] = v
	}
	n.requestCallbacks = make(map[string]ConsensusCallback)
	n.callbackMu.Unlock()

	// Invoke all callbacks with failure
	for requestID, callback := range callbacks {
		if callback != nil {
			go callback(false, fmt.Errorf("consensus failed: %s (request: %s)", reason, requestID))
		}
	}
}

// processMessages processes incoming consensus messages
func (n *PBFTNode) processMessages() {
	fmt.Printf("PBFT: Node %s processMessages goroutine started\n", n.nodeID)
	for {
		select {
		case <-n.ctx.Done():
			fmt.Printf("PBFT: Node %s processMessages goroutine stopping (context cancelled)\n", n.nodeID)
			return
		case msg := <-n.messageQueue:
			// Process based on message type
			switch m := msg.(type) {
			case *PrePrepareMsg:
				fmt.Printf("PBFT: Node %s processing PRE_PREPARE from queue for seq %d\n", n.nodeID, m.Sequence)
				if err := n.handlePrePrepare(m); err != nil {
					fmt.Printf("Error handling PRE-PREPARE: %v\n", err)
				}
			case *PrepareMsg:
				fmt.Printf("PBFT: Node %s processing PREPARE from queue (from %s for seq %d)\n", n.nodeID, m.NodeID, m.Sequence)
				if err := n.HandlePrepare(m); err != nil {
					fmt.Printf("Error handling PREPARE: %v\n", err)
				}
			case *CommitMsg:
				fmt.Printf("PBFT: Node %s processing COMMIT from queue (from %s for seq %d)\n", n.nodeID, m.NodeID, m.Sequence)
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

	// Only trigger view change if there's a pending consensus operation
	// If the node is idle, just reset the timer and continue
	if n.state == StateIdle {
		n.viewTimer.Reset(n.viewTimeout)
		n.mu.Unlock()
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
	n.viewTimer.Reset(n.viewTimeout)

	n.mu.Unlock()

	// Fail all pending callbacks - consensus did not complete
	n.failPendingCallbacks("view timeout")
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

	// Map message type to name for logging
	msgTypeName := map[int32]string{
		0: "PRE_PREPARE",
		1: "PREPARE",
		2: "COMMIT",
		7: "CLIENT_REQUEST",
	}[msgType]
	if msgTypeName == "" {
		msgTypeName = fmt.Sprintf("UNKNOWN(%d)", msgType)
	}

	fmt.Printf("PBFT: Node %s HandleNetworkMessage called with type %s (payload size: %d bytes)\n",
		n.nodeID, msgTypeName, len(payload))

	// Deserialize based on message type
	switch msgType {
	case 0: // PRE_PREPARE
		msg, err = DeserializePrePrepare(payload)
		if err == nil {
			prePrepare := msg.(*PrePrepareMsg)
			fmt.Printf("PBFT: Node %s deserialized PRE_PREPARE for seq %d, view %d\n",
				n.nodeID, prePrepare.Sequence, prePrepare.View)
		}
	case 1: // PREPARE
		msg, err = DeserializePrepare(payload)
		if err == nil {
			prepare := msg.(*PrepareMsg)
			fmt.Printf("PBFT: Node %s deserialized PREPARE from %s for seq %d, view %d\n",
				n.nodeID, prepare.NodeID, prepare.Sequence, prepare.View)
		}
	case 2: // COMMIT
		msg, err = DeserializeCommit(payload)
		if err == nil {
			commit := msg.(*CommitMsg)
			fmt.Printf("PBFT: Node %s deserialized COMMIT from %s for seq %d, view %d\n",
				n.nodeID, commit.NodeID, commit.Sequence, commit.View)
		}
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
		fmt.Printf("PBFT: Node %s failed to deserialize %s message: %v\n", n.nodeID, msgTypeName, err)
		return fmt.Errorf("failed to deserialize message: %w", err)
	}

	// Inject into message queue
	select {
	case n.messageQueue <- msg:
		fmt.Printf("PBFT: Node %s successfully queued %s message for processing\n", n.nodeID, msgTypeName)
		return nil
	case <-n.ctx.Done():
		return n.ctx.Err()
	default:
		fmt.Printf("PBFT: Node %s message queue FULL, dropping %s message!\n", n.nodeID, msgTypeName)
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
