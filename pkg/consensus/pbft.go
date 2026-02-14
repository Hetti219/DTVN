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

// checkpointInterval defines how many operations between checkpoints
const checkpointInterval int64 = 10

// PBFTNode represents a PBFT consensus node
type PBFTNode struct {
	nodeID               string
	view                 int64
	sequence             int64
	state                State
	f                    int // Maximum number of Byzantine nodes tolerated
	totalNodes           int
	isPrimary            bool
	prepareLog           map[int64]map[string]*PrepareMsg
	commitLog            map[int64]map[string]*CommitMsg
	requestLog           map[int64]*Request
	messageQueue         chan ConsensusMessage
	viewTimer            *time.Timer
	viewTimeout          time.Duration // Configurable view timeout for consensus
	ctx                  context.Context
	cancel               context.CancelFunc
	mu                   sync.RWMutex
	handlers             []ConsensusHandler
	broadcaster          MessageBroadcaster
	checkpoints          map[int64]*Checkpoint
	checkpointVotes      map[int64]map[string]*Checkpoint
	lastStableCheckpoint int64
	viewChangeLog        map[int64]map[string]*ViewChangeMsg

	// Tracks whether this non-primary node has a pending client request
	// that the primary hasn't started consensus for. Enables view change
	// from IDLE state when the primary appears to be dead.
	hasPendingClientRequest bool

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
	NodeID    string // Originating node ID, carried through consensus via ClientSignature proto field
}

// Checkpoint represents a state checkpoint
type Checkpoint struct {
	Sequence    int64
	StateDigest string
	NodeID      string
	Timestamp   int64
}

// ViewChangeMsg represents a VIEW_CHANGE message
type ViewChangeMsg struct {
	NewView int64
	NodeID  string
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
		// Default to 15 seconds to allow sufficient time for PBFT three-phase commit across network latency.
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
		checkpointVotes:  make(map[int64]map[string]*Checkpoint),
		viewChangeLog:    make(map[int64]map[string]*ViewChangeMsg),
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

	n.requestLog[msg.Sequence] = msg.Request

	// Reset view timer for this new consensus round.
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

	// Also check if COMMITs arrived early (before PRE-PREPARE) and we already have commit quorum.
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
	if n.stateReplicator != nil {
		if err := n.stateReplicator(req); err != nil {
			// Log the error but don't fail - state was applied locally and gossip will eventually propagate it via anti-entropy
			fmt.Printf("PBFT: Warning - State replication failed (will retry via anti-entropy): %v\n", err)
		} else {
			fmt.Printf("PBFT: ✅ State replicated to all peers for ticket %s\n", req.TicketID)
		}
	}

	// Invoke callback with success
	n.invokeCallback(req.RequestID, true, nil)

	// Reset to idle
	n.state = StateIdle
	n.hasPendingClientRequest = false

	// Reset view timer
	n.viewTimer.Reset(n.viewTimeout)

	// Create checkpoint at interval to enable log garbage collection
	if sequence%checkpointInterval == 0 {
		n.createCheckpoint(sequence)
	}

	return nil
}

// RegisterHandler registers a consensus handler
func (n *PBFTNode) RegisterHandler(handler ConsensusHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers = append(n.handlers, handler)
}

// RegisterCallback registers a callback for a specific request that will be called when consensus completes (success or failure).
func (n *PBFTNode) RegisterCallback(requestID string, callback ConsensusCallback) {
	n.callbackMu.Lock()
	defer n.callbackMu.Unlock()
	n.requestCallbacks[requestID] = callback
}

// RegisterStateReplicator registers a callback that is invoked after consensus commits to ensure reliable state propagation to all peers.
func (n *PBFTNode) RegisterStateReplicator(replicator StateReplicator) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stateReplicator = replicator
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
	// or a pending client request waiting for the primary to act.
	if n.state == StateIdle && !n.hasPendingClientRequest {
		n.viewTimer.Reset(n.viewTimeout)
		n.mu.Unlock()
		return
	}

	// Clear the pending request flag - we are handling it via view change
	n.hasPendingClientRequest = false

	// There's a pending request that hasn't completed - trigger view change
	fmt.Printf("Node %s: View timeout while in state %s, initiating view change\n", n.nodeID, n.state)

	n.view++
	newView := n.view

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

	// Record own view change vote
	if n.viewChangeLog[newView] == nil {
		n.viewChangeLog[newView] = make(map[string]*ViewChangeMsg)
	}
	n.viewChangeLog[newView][n.nodeID] = &ViewChangeMsg{
		NewView: newView,
		NodeID:  n.nodeID,
	}

	n.mu.Unlock()

	// Broadcast VIEW_CHANGE message to other nodes
	payload, err := SerializeViewChange(&ViewChangeMsg{
		NewView: newView,
		NodeID:  n.nodeID,
	})
	if err != nil {
		fmt.Printf("Failed to serialize VIEW_CHANGE: %v\n", err)
	} else {
		data, err := SerializePBFTMessage(3, payload, n.nodeID) // 3 = VIEW_CHANGE type
		if err != nil {
			fmt.Printf("Failed to wrap VIEW_CHANGE message: %v\n", err)
		} else {
			go func() {
				ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
				defer cancel()
				if err := n.broadcaster.Broadcast(ctx, data); err != nil {
					fmt.Printf("Failed to broadcast VIEW_CHANGE: %v\n", err)
				}
			}()
		}
	}

	// Fail all pending callbacks - consensus did not complete
	n.failPendingCallbacks("view timeout")
}

// HandleViewChange handles incoming VIEW_CHANGE messages from other nodes.
// When 2f+1 nodes agree on a new view, this node transitions to that view.
func (n *PBFTNode) HandleViewChange(msg *ViewChangeMsg) error {
	n.mu.Lock()

	fmt.Printf("PBFT: Node %s received VIEW_CHANGE from %s for new view %d (current view: %d)\n",
		n.nodeID, msg.NodeID, msg.NewView, n.view)

	// Ignore view changes for views we've already passed
	if msg.NewView <= n.view {
		n.mu.Unlock()
		return nil
	}

	// Store the view change vote
	if n.viewChangeLog[msg.NewView] == nil {
		n.viewChangeLog[msg.NewView] = make(map[string]*ViewChangeMsg)
	}
	n.viewChangeLog[msg.NewView][msg.NodeID] = msg

	// Check if we have 2f+1 view change messages for this new view
	required := 2*n.f + 1
	if required < 1 {
		required = 1
	}
	current := len(n.viewChangeLog[msg.NewView])

	fmt.Printf("PBFT: Node %s VIEW_CHANGE progress for view %d: %d/%d\n",
		n.nodeID, msg.NewView, current, required)

	if current < required {
		n.mu.Unlock()
		return nil
	}

	// Quorum reached — transition to new view
	fmt.Printf("PBFT: Node %s has VIEW_CHANGE quorum for view %d, transitioning\n",
		n.nodeID, msg.NewView)

	n.view = msg.NewView

	// Calculate new primary
	primaryIndex := n.view % int64(n.totalNodes)
	expectedPrimaryID := fmt.Sprintf("node%d", primaryIndex)
	n.isPrimary = (n.nodeID == expectedPrimaryID)

	// Reset state
	n.state = StateIdle
	n.hasPendingClientRequest = false
	n.viewTimer.Reset(n.viewTimeout)

	// Clean up old view change logs
	for v := range n.viewChangeLog {
		if v <= n.view {
			delete(n.viewChangeLog, v)
		}
	}

	fmt.Printf("PBFT: Node %s transitioned to view %d, primary: %s, isPrimary: %v\n",
		n.nodeID, n.view, expectedPrimaryID, n.isPrimary)

	n.mu.Unlock()

	// Fail pending callbacks outside the lock — old view consensus won't complete
	n.failPendingCallbacks("view change")

	return nil
}

// HandleCheckpoint handles incoming CHECKPOINT messages from other nodes.
func (n *PBFTNode) HandleCheckpoint(ckpt *Checkpoint) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	fmt.Printf("PBFT: Node %s received CHECKPOINT from %s for seq %d\n",
		n.nodeID, ckpt.NodeID, ckpt.Sequence)

	// Ignore checkpoints at or below our last stable checkpoint
	if ckpt.Sequence <= n.lastStableCheckpoint {
		return nil
	}

	// Store checkpoint vote
	if n.checkpointVotes[ckpt.Sequence] == nil {
		n.checkpointVotes[ckpt.Sequence] = make(map[string]*Checkpoint)
	}
	n.checkpointVotes[ckpt.Sequence][ckpt.NodeID] = ckpt

	// Check for stable checkpoint (2f+1 matching checkpoints)
	required := 2*n.f + 1
	if required < 1 {
		required = 1
	}
	votes := n.checkpointVotes[ckpt.Sequence]

	if len(votes) < required {
		return nil
	}

	// Count votes per digest to verify agreement
	digests := make(map[string]int)
	for _, v := range votes {
		digests[v.StateDigest]++
	}

	// Establish stable checkpoint if any digest has quorum
	for digest, count := range digests {
		if count >= required {
			fmt.Printf("PBFT: Node %s stable checkpoint at seq %d (digest: %s)\n",
				n.nodeID, ckpt.Sequence, digest)

			n.lastStableCheckpoint = ckpt.Sequence
			n.garbageCollect(ckpt.Sequence)

			// Clean up old checkpoint votes
			for seq := range n.checkpointVotes {
				if seq <= ckpt.Sequence {
					delete(n.checkpointVotes, seq)
				}
			}
			break
		}
	}

	return nil
}

// createCheckpoint creates and broadcasts a checkpoint at the given sequence.
func (n *PBFTNode) createCheckpoint(sequence int64) {
	// Compute state digest as hash of the sequence number.
	digest := fmt.Sprintf("%d", sequence)
	hash := sha256.Sum256([]byte(digest))
	stateDigest := hex.EncodeToString(hash[:])

	ckpt := &Checkpoint{
		Sequence:    sequence,
		StateDigest: stateDigest,
		NodeID:      n.nodeID,
		Timestamp:   time.Now().UnixNano(),
	}

	// Store own checkpoint
	n.checkpoints[sequence] = ckpt

	// Add own vote for checkpoint quorum
	if n.checkpointVotes[sequence] == nil {
		n.checkpointVotes[sequence] = make(map[string]*Checkpoint)
	}
	n.checkpointVotes[sequence][n.nodeID] = ckpt

	fmt.Printf("PBFT: Node %s created checkpoint at seq %d\n", n.nodeID, sequence)

	// Broadcast checkpoint (goroutine because caller holds the lock)
	payload, err := SerializeCheckpoint(ckpt)
	if err != nil {
		fmt.Printf("Failed to serialize CHECKPOINT: %v\n", err)
		return
	}
	data, err := SerializePBFTMessage(4, payload, n.nodeID) // 4 = CHECKPOINT type
	if err != nil {
		fmt.Printf("Failed to wrap CHECKPOINT message: %v\n", err)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(n.ctx, 3*time.Second)
		defer cancel()
		if err := n.broadcaster.Broadcast(ctx, data); err != nil {
			fmt.Printf("Failed to broadcast CHECKPOINT: %v\n", err)
		}
	}()
}

// garbageCollect removes old consensus log entries up to the stable checkpoint.
func (n *PBFTNode) garbageCollect(upToSequence int64) {
	for seq := range n.requestLog {
		if seq <= upToSequence {
			delete(n.requestLog, seq)
		}
	}
	for seq := range n.prepareLog {
		if seq <= upToSequence {
			delete(n.prepareLog, seq)
		}
	}
	for seq := range n.commitLog {
		if seq <= upToSequence {
			delete(n.commitLog, seq)
		}
	}
	for seq := range n.checkpoints {
		if seq < upToSequence {
			delete(n.checkpoints, seq)
		}
	}

	fmt.Printf("PBFT: Node %s garbage collected logs up to seq %d\n", n.nodeID, upToSequence)
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

// NotifyPendingRequest marks that a client request has been forwarded to the
// primary but no consensus has started.
func (n *PBFTNode) NotifyPendingRequest() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hasPendingClientRequest = true
	n.viewTimer.Reset(n.viewTimeout)
}

// ClearPendingRequest clears the pending client request flag, called when the request completes successfully or after a view change.
func (n *PBFTNode) ClearPendingRequest() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hasPendingClientRequest = false
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
		3: "VIEW_CHANGE",
		4: "CHECKPOINT",
		6: "HEARTBEAT",
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
	case 3: // VIEW_CHANGE
		viewChangeMsg, err := DeserializeViewChange(payload)
		if err != nil {
			return fmt.Errorf("failed to deserialize view change: %w", err)
		}
		return n.HandleViewChange(viewChangeMsg)
	case 4: // CHECKPOINT
		ckpt, err := DeserializeCheckpoint(payload)
		if err != nil {
			return fmt.Errorf("failed to deserialize checkpoint: %w", err)
		}
		return n.HandleCheckpoint(ckpt)
	case 6: // HEARTBEAT
		fmt.Printf("PBFT: Node %s received HEARTBEAT (payload size: %d bytes)\n", n.nodeID, len(payload))
		return nil
	case 7: // CLIENT_REQUEST - forwarded from non-primary node
		req, err := DeserializeRequest(payload)
		if err != nil {
			return fmt.Errorf("failed to deserialize client request: %w", err)
		}

		if !n.IsPrimary() {
			// Non-primary: track the request so the view timer can trigger a
			// view change if the primary never starts consensus (dead primary).
			fmt.Printf("PBFT: Non-primary %s received CLIENT_REQUEST for ticket %s, tracking for view change\n",
				n.nodeID, req.TicketID)
			n.NotifyPendingRequest()
			return nil
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
