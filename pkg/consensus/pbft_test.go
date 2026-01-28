package consensus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockBroadcaster implements MessageBroadcaster for testing
type MockBroadcaster struct {
	mu           sync.Mutex
	messages     [][]byte
	peers        int
	broadcastErr error
	sendToErr    error
}

func NewMockBroadcaster(peerCount int) *MockBroadcaster {
	return &MockBroadcaster{
		messages: make([][]byte, 0),
		peers:    peerCount,
	}
}

func (m *MockBroadcaster) Broadcast(ctx context.Context, data []byte) error {
	if m.broadcastErr != nil {
		return m.broadcastErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, data)
	return nil
}

func (m *MockBroadcaster) SendTo(ctx context.Context, nodeID string, data []byte) error {
	if m.sendToErr != nil {
		return m.sendToErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, data)
	return nil
}

func (m *MockBroadcaster) GetPeerCount() int {
	return m.peers
}

func (m *MockBroadcaster) GetMessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func (m *MockBroadcaster) SetPeerCount(count int) {
	m.peers = count
}

// TestNewPBFTNode tests PBFT node creation
func TestNewPBFTNode(t *testing.T) {
	t.Run("CreatePrimaryNode", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:      "node0",
			TotalNodes:  3,
			IsPrimary:   true,
			ViewTimeout: 5 * time.Second,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		require.NotNil(t, node)
		defer node.Close()

		assert.Equal(t, "node0", node.nodeID)
		assert.True(t, node.isPrimary)
		assert.Equal(t, 3, node.totalNodes)
		assert.Equal(t, 0, node.f) // (3-1)/3 = 0
		assert.Equal(t, int64(0), node.view)
		assert.Equal(t, int64(0), node.sequence)
		assert.Equal(t, StateIdle, node.state)
	})

	t.Run("CreateReplicaNode", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 3,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		assert.Equal(t, "node1", node.nodeID)
		assert.False(t, node.isPrimary)
	})

	t.Run("CalculateFCorrectly", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(8)

		testCases := []struct {
			totalNodes int
			expectedF  int
		}{
			{1, 0},  // (1-1)/3 = 0
			{3, 0},  // (3-1)/3 = 0
			{4, 1},  // (4-1)/3 = 1
			{7, 2},  // (7-1)/3 = 2
			{10, 3}, // (10-1)/3 = 3
		}

		for _, tc := range testCases {
			cfg := &Config{
				NodeID:     "test-node",
				TotalNodes: tc.totalNodes,
				IsPrimary:  false,
			}

			node, err := NewPBFTNode(ctx, cfg, broadcaster)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedF, node.f, "Failed for totalNodes=%d", tc.totalNodes)
			node.Close()
		}
	})

	t.Run("DefaultViewTimeout", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
			// ViewTimeout not set
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		assert.NotNil(t, node.viewTimer)
	})

	t.Run("CustomViewTimeout", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		customTimeout := 10 * time.Second
		cfg := &Config{
			NodeID:      "node0",
			TotalNodes:  3,
			IsPrimary:   true,
			ViewTimeout: customTimeout,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		assert.NotNil(t, node.viewTimer)
	})
}

// TestProposeRequest tests request proposal
func TestProposeRequest(t *testing.T) {
	t.Run("PrimaryProposesRequest", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2) // Sufficient for quorum with totalNodes=3

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		node.Start()

		req := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: time.Now().Unix(),
		}

		err = node.ProposeRequest(req)
		require.NoError(t, err)

		// Give time for async broadcast
		time.Sleep(100 * time.Millisecond)

		// Verify sequence incremented
		assert.Equal(t, int64(1), node.GetSequence())

		// Verify request stored
		node.mu.RLock()
		storedReq := node.requestLog[1]
		node.mu.RUnlock()
		assert.Equal(t, req, storedReq)
	})

	t.Run("ReplicaCannotPropose", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 3,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		req := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: time.Now().Unix(),
		}

		err = node.ProposeRequest(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only primary can propose")
	})

	t.Run("SingleNodeMode", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(0) // No peers

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 1,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		node.Start()

		handlerCalled := false
		node.RegisterHandler(func(req *Request) error {
			handlerCalled = true
			return nil
		})

		req := &Request{
			RequestID: "req-single",
			TicketID:  "ticket-single",
			Operation: "VALIDATE",
			Timestamp: time.Now().Unix(),
		}

		err = node.ProposeRequest(req)
		require.NoError(t, err)

		// Give time for processing
		time.Sleep(100 * time.Millisecond)

		assert.True(t, handlerCalled)
	})

	t.Run("InsufficientPeersForQuorum", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(0) // No peers

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 4, // Needs 3 nodes for quorum
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		req := &Request{
			RequestID: "req-fail",
			TicketID:  "ticket-fail",
			Operation: "VALIDATE",
			Timestamp: time.Now().Unix(),
		}

		err = node.ProposeRequest(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient peers")
	})

	t.Run("MultipleSequentialProposals", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		node.Start()

		for i := 1; i <= 5; i++ {
			req := &Request{
				RequestID: "req-" + string(rune('0'+i)),
				TicketID:  "ticket-" + string(rune('0'+i)),
				Operation: "VALIDATE",
				Timestamp: time.Now().Unix(),
			}

			err := node.ProposeRequest(req)
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond)
		}

		assert.Equal(t, int64(5), node.GetSequence())
	})
}

// TestMessageHandling tests message processing
func TestMessageHandling(t *testing.T) {
	t.Run("HandlePrePrepare", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		// Use TotalNodes=10 so quorum=7; state stays at PREPARE after one message
		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 10,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		node.Start()

		req := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: time.Now().Unix(),
		}

		digest := node.computeDigest(req)

		prePrepare := &PrePrepareMsg{
			View:     0,
			Sequence: 1,
			Digest:   digest,
			Request:  req,
		}

		err = node.handlePrePrepare(prePrepare)
		require.NoError(t, err)

		// Verify state changed to PREPARE
		node.mu.RLock()
		state := node.state
		node.mu.RUnlock()
		assert.Equal(t, StatePrepare, state)
	})

	t.Run("HandlePrepare", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 3,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Seed requestLog so executeRequest succeeds if quorum is reached
		node.mu.Lock()
		node.requestLog[1] = &Request{
			RequestID: "test-request",
			TicketID:  "test-ticket",
			Operation: "VALIDATE",
		}
		node.mu.Unlock()

		prepare := &PrepareMsg{
			View:     0,
			Sequence: 1,
			Digest:   "test-digest",
			NodeID:   "node0",
		}

		err = node.HandlePrepare(prepare)
		require.NoError(t, err)

		// Verify prepare stored
		node.mu.RLock()
		storedPrepare := node.prepareLog[1]["node0"]
		node.mu.RUnlock()
		assert.NotNil(t, storedPrepare)
	})

	t.Run("HandleCommit", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 3,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Seed requestLog so executeRequest succeeds if quorum is reached
		node.mu.Lock()
		node.requestLog[1] = &Request{
			RequestID: "test-request",
			TicketID:  "test-ticket",
			Operation: "VALIDATE",
		}
		node.mu.Unlock()

		commit := &CommitMsg{
			View:     0,
			Sequence: 1,
			Digest:   "test-digest",
			NodeID:   "node0",
		}

		err = node.HandleCommit(commit)
		require.NoError(t, err)

		// Verify commit stored
		node.mu.RLock()
		storedCommit := node.commitLog[1]["node0"]
		node.mu.RUnlock()
		assert.NotNil(t, storedCommit)
	})

	t.Run("RejectMismatchedView", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 3,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Set node to view 1
		node.mu.Lock()
		node.view = 1
		node.mu.Unlock()

		// Try to handle message from view 0
		prepare := &PrepareMsg{
			View:     0,
			Sequence: 1,
			Digest:   "test-digest",
			NodeID:   "node0",
		}

		err = node.HandlePrepare(prepare)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "view mismatch")
	})

	t.Run("RejectInvalidDigest", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node1",
			TotalNodes: 3,
			IsPrimary:  false,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		req := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: time.Now().Unix(),
		}

		prePrepare := &PrePrepareMsg{
			View:     0,
			Sequence: 1,
			Digest:   "wrong-digest",
			Request:  req,
		}

		err = node.handlePrePrepare(prePrepare)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "digest mismatch")
	})
}

// TestQuorumChecks tests quorum verification
func TestQuorumChecks(t *testing.T) {
	t.Run("PrepareQuorumReached", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 4, // f=1, quorum=3
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Add 3 PREPARE messages
		node.mu.Lock()
		node.prepareLog[1] = make(map[string]*PrepareMsg)
		node.prepareLog[1]["node0"] = &PrepareMsg{NodeID: "node0"}
		node.prepareLog[1]["node1"] = &PrepareMsg{NodeID: "node1"}
		node.prepareLog[1]["node2"] = &PrepareMsg{NodeID: "node2"}
		node.mu.Unlock()

		hasQuorum := node.checkPrepareQuorum(1)
		assert.True(t, hasQuorum)
	})

	t.Run("PrepareQuorumNotReached", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 4, // f=1, quorum=3
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Add only 2 PREPARE messages
		node.mu.Lock()
		node.prepareLog[1] = make(map[string]*PrepareMsg)
		node.prepareLog[1]["node0"] = &PrepareMsg{NodeID: "node0"}
		node.prepareLog[1]["node1"] = &PrepareMsg{NodeID: "node1"}
		node.mu.Unlock()

		hasQuorum := node.checkPrepareQuorum(1)
		assert.False(t, hasQuorum)
	})

	t.Run("CommitQuorumReached", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 4, // f=1, quorum=3
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Add 3 COMMIT messages
		node.mu.Lock()
		node.commitLog[1] = make(map[string]*CommitMsg)
		node.commitLog[1]["node0"] = &CommitMsg{NodeID: "node0"}
		node.commitLog[1]["node1"] = &CommitMsg{NodeID: "node1"}
		node.commitLog[1]["node2"] = &CommitMsg{NodeID: "node2"}
		node.mu.Unlock()

		hasQuorum := node.checkCommitQuorum(1)
		assert.True(t, hasQuorum)
	})

	t.Run("SingleNodeQuorum", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(0)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 1, // f=0, quorum=1
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Single PREPARE message should be enough
		node.mu.Lock()
		node.prepareLog[1] = make(map[string]*PrepareMsg)
		node.prepareLog[1]["node0"] = &PrepareMsg{NodeID: "node0"}
		node.mu.Unlock()

		hasQuorum := node.checkPrepareQuorum(1)
		assert.True(t, hasQuorum)
	})
}

// TestHandlerRegistration tests consensus handler registration
func TestHandlerRegistration(t *testing.T) {
	t.Run("RegisterSingleHandler", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		handler := func(req *Request) error {
			return nil
		}

		node.RegisterHandler(handler)

		node.mu.RLock()
		handlerCount := len(node.handlers)
		node.mu.RUnlock()

		assert.Equal(t, 1, handlerCount)
	})

	t.Run("RegisterMultipleHandlers", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		for i := 0; i < 3; i++ {
			node.RegisterHandler(func(req *Request) error {
				return nil
			})
		}

		node.mu.RLock()
		handlerCount := len(node.handlers)
		node.mu.RUnlock()

		assert.Equal(t, 3, handlerCount)
	})
}

// TestViewChange tests view change functionality
func TestViewChange(t *testing.T) {
	t.Run("ViewChangeFromIdle", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:      "node0",
			TotalNodes:  3,
			IsPrimary:   true,
			ViewTimeout: 100 * time.Millisecond,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Node is idle, view should not change
		initialView := node.GetView()

		// Manually trigger view change
		node.initiateViewChange()

		// View should remain the same because state is idle
		assert.Equal(t, initialView, node.GetView())
	})

	t.Run("ViewChangeFromPendingState", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Set node to non-idle state
		node.mu.Lock()
		node.state = StatePrepare
		node.mu.Unlock()

		initialView := node.GetView()

		// Trigger view change
		node.initiateViewChange()

		// View should increment
		assert.Equal(t, initialView+1, node.GetView())

		// State should reset to idle
		node.mu.RLock()
		state := node.state
		node.mu.RUnlock()
		assert.Equal(t, StateIdle, state)
	})

	t.Run("PrimaryRotation", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// Initial primary
		assert.Equal(t, "node0", node.GetPrimary())

		// Set non-idle state and trigger view change
		node.mu.Lock()
		node.state = StatePrepare
		node.mu.Unlock()

		node.initiateViewChange()

		// Primary should rotate to node1
		assert.Equal(t, "node1", node.GetPrimary())
	})
}

// TestGetters tests getter methods
func TestGetters(t *testing.T) {
	t.Run("GetView", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		assert.Equal(t, int64(0), node.GetView())

		node.mu.Lock()
		node.view = 5
		node.mu.Unlock()

		assert.Equal(t, int64(5), node.GetView())
	})

	t.Run("GetSequence", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		assert.Equal(t, int64(0), node.GetSequence())

		node.mu.Lock()
		node.sequence = 10
		node.mu.Unlock()

		assert.Equal(t, int64(10), node.GetSequence())
	})

	t.Run("IsPrimary", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		assert.True(t, node.IsPrimary())

		node.mu.Lock()
		node.isPrimary = false
		node.mu.Unlock()

		assert.False(t, node.IsPrimary())
	})

	t.Run("GetPrimary", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		// View 0 -> node0
		assert.Equal(t, "node0", node.GetPrimary())

		// View 1 -> node1
		node.mu.Lock()
		node.view = 1
		node.mu.Unlock()
		assert.Equal(t, "node1", node.GetPrimary())

		// View 2 -> node2
		node.mu.Lock()
		node.view = 2
		node.mu.Unlock()
		assert.Equal(t, "node2", node.GetPrimary())

		// View 3 -> node0 (wraps around)
		node.mu.Lock()
		node.view = 3
		node.mu.Unlock()
		assert.Equal(t, "node0", node.GetPrimary())
	})
}

// TestComputeDigest tests digest computation
func TestComputeDigest(t *testing.T) {
	t.Run("SameRequestSameDigest", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		req := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: 12345,
		}

		digest1 := node.computeDigest(req)
		digest2 := node.computeDigest(req)

		assert.Equal(t, digest1, digest2)
	})

	t.Run("DifferentRequestDifferentDigest", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		req1 := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: 12345,
		}

		req2 := &Request{
			RequestID: "req-002",
			TicketID:  "ticket-002",
			Operation: "VALIDATE",
			Timestamp: 12345,
		}

		digest1 := node.computeDigest(req1)
		digest2 := node.computeDigest(req2)

		assert.NotEqual(t, digest1, digest2)
	})

	t.Run("DigestIsHex", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		req := &Request{
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Timestamp: 12345,
		}

		digest := node.computeDigest(req)

		// Should be 64 hex characters (SHA256)
		assert.Len(t, digest, 64)
	})
}

// TestNodeClose tests node shutdown
func TestNodeClose(t *testing.T) {
	t.Run("CloseNode", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)

		err = node.Close()
		assert.NoError(t, err)

		// Context should be cancelled
		select {
		case <-node.ctx.Done():
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Context was not cancelled on Close()")
		}
	})
}

// TestConcurrentOperations tests thread safety
func TestConcurrentOperations(t *testing.T) {
	t.Run("ConcurrentHandlerRegistration", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				node.RegisterHandler(func(req *Request) error {
					return nil
				})
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		node.mu.RLock()
		count := len(node.handlers)
		node.mu.RUnlock()

		assert.Equal(t, 10, count)
	})

	t.Run("ConcurrentGetters", func(t *testing.T) {
		ctx := context.Background()
		broadcaster := NewMockBroadcaster(2)

		cfg := &Config{
			NodeID:     "node0",
			TotalNodes: 3,
			IsPrimary:  true,
		}

		node, err := NewPBFTNode(ctx, cfg, broadcaster)
		require.NoError(t, err)
		defer node.Close()

		done := make(chan bool)
		for i := 0; i < 20; i++ {
			go func() {
				_ = node.GetView()
				_ = node.GetSequence()
				_ = node.IsPrimary()
				_ = node.GetPrimary()
				done <- true
			}()
		}

		for i := 0; i < 20; i++ {
			<-done
		}
	})
}
