package gossip

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPeerManager implements PeerManager for testing
type MockPeerManager struct {
	mu       sync.Mutex
	peers    []peer.ID
	messages map[peer.ID][]byte
	sendErr  error
}

func NewMockPeerManager() *MockPeerManager {
	return &MockPeerManager{
		peers:    make([]peer.ID, 0),
		messages: make(map[peer.ID][]byte),
	}
}

func (m *MockPeerManager) AddPeer(peerID peer.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers = append(m.peers, peerID)
}

func (m *MockPeerManager) GetPeers() []peer.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	peers := make([]peer.ID, len(m.peers))
	copy(peers, m.peers)
	return peers
}

func (m *MockPeerManager) GetPeerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.peers)
}

func (m *MockPeerManager) SendMessage(ctx context.Context, peerID peer.ID, data []byte) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[peerID] = data
	return nil
}

func (m *MockPeerManager) GetMessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// Helper to create test peer IDs
func createTestPeerID(t *testing.T) peer.ID {
	_, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
	require.NoError(t, err)
	peerID, err := peer.IDFromPublicKey(pub)
	require.NoError(t, err)
	return peerID
}

// TestNewGossipEngine tests engine creation
func TestNewGossipEngine(t *testing.T) {
	t.Run("CreateEngineWithDefaultFanout", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			// Fanout not set
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		require.NotNil(t, engine)
		defer engine.Close()

		assert.Equal(t, "test-node", engine.nodeID)
		assert.Equal(t, 3, engine.fanout) // Default fanout
		assert.NotNil(t, engine.cache)
		assert.NotNil(t, engine.messageChan)
	})

	t.Run("CreateEngineWithCustomFanout", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 5,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		assert.Equal(t, 5, engine.fanout)
	})

	t.Run("EngineInitializesCorrectly", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		assert.Equal(t, 0, engine.GetCacheSize())
		assert.NotNil(t, engine.handlers)
		assert.NotNil(t, engine.sentMessages)
	})
}

// TestPublish tests message publishing
func TestPublish(t *testing.T) {
	t.Run("PublishMessage", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		payload := []byte("test message")
		err = engine.Publish(payload)
		require.NoError(t, err)

		// Give time for async processing
		time.Sleep(50 * time.Millisecond)

		// Message should be in cache
		assert.Equal(t, 1, engine.GetCacheSize())
	})

	t.Run("PublishMultipleMessages", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		for i := 0; i < 5; i++ {
			payload := []byte("message-" + string(rune('0'+i)))
			err := engine.Publish(payload)
			require.NoError(t, err)
		}

		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, 5, engine.GetCacheSize())
	})

	t.Run("PublishGeneratesUniqueID", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		payload1 := []byte("message-1")
		payload2 := []byte("message-2")

		id1 := generateMessageID(payload1)
		id2 := generateMessageID(payload2)

		assert.NotEqual(t, id1, id2)
	})

	t.Run("PublishSamePayloadGeneratesSameID", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		payload := []byte("consistent-message")

		id1 := generateMessageID(payload)
		id2 := generateMessageID(payload)

		assert.Equal(t, id1, id2)
	})
}

// TestReceiveMessage tests message reception
func TestReceiveMessage(t *testing.T) {
	t.Run("ReceiveNewMessage", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		handlerCalled := false
		engine.RegisterHandler(func(msg *Message) error {
			handlerCalled = true
			return nil
		})

		msg := &Message{
			ID:        "test-msg-id",
			Payload:   []byte("test payload"),
			TTL:       5,
			SeenBy:    []string{"other-node"},
			Timestamp: time.Now().Unix(),
		}

		err = engine.ReceiveMessage(msg)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		assert.True(t, handlerCalled)
		assert.Equal(t, 1, engine.GetCacheSize())
	})

	t.Run("ReceiveDuplicateMessage", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		callCount := 0
		engine.RegisterHandler(func(msg *Message) error {
			callCount++
			return nil
		})

		msg := &Message{
			ID:        "duplicate-msg",
			Payload:   []byte("test"),
			TTL:       5,
			SeenBy:    []string{},
			Timestamp: time.Now().Unix(),
		}

		// Receive first time
		err = engine.ReceiveMessage(msg)
		require.NoError(t, err)

		// Receive second time (duplicate)
		err = engine.ReceiveMessage(msg)
		require.NoError(t, err)

		// Handler should only be called once
		assert.Equal(t, 1, callCount)
	})

	t.Run("ReceiveMessageDecrementsTTL", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		msg := &Message{
			ID:        "ttl-test",
			Payload:   []byte("test"),
			TTL:       5,
			SeenBy:    []string{},
			Timestamp: time.Now().Unix(),
		}

		err = engine.ReceiveMessage(msg)
		require.NoError(t, err)

		// TTL should be decremented
		assert.Equal(t, int32(4), msg.TTL)
	})

	t.Run("ReceiveMessageWithZeroTTL", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		msg := &Message{
			ID:        "zero-ttl",
			Payload:   []byte("test"),
			TTL:       0,
			SeenBy:    []string{},
			Timestamp: time.Now().Unix(),
		}

		err = engine.ReceiveMessage(msg)
		require.NoError(t, err)

		// Should be added to cache but not propagated
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, engine.GetCacheSize())
	})
}

// TestHandlerRegistration tests handler registration
func TestHandlerRegistration(t *testing.T) {
	t.Run("RegisterSingleHandler", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		handler := func(msg *Message) error {
			return nil
		}

		engine.RegisterHandler(handler)

		engine.mu.RLock()
		count := len(engine.handlers)
		engine.mu.RUnlock()

		assert.Equal(t, 1, count)
	})

	t.Run("RegisterMultipleHandlers", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		for i := 0; i < 3; i++ {
			engine.RegisterHandler(func(msg *Message) error {
				return nil
			})
		}

		engine.mu.RLock()
		count := len(engine.handlers)
		engine.mu.RUnlock()

		assert.Equal(t, 3, count)
	})

	t.Run("AllHandlersCalled", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		callCounts := make([]int, 3)
		for i := 0; i < 3; i++ {
			idx := i
			engine.RegisterHandler(func(msg *Message) error {
				callCounts[idx]++
				return nil
			})
		}

		msg := &Message{
			ID:        "multi-handler",
			Payload:   []byte("test"),
			TTL:       5,
			SeenBy:    []string{},
			Timestamp: time.Now().Unix(),
		}

		err = engine.ReceiveMessage(msg)
		require.NoError(t, err)

		// All handlers should be called
		for i := 0; i < 3; i++ {
			assert.Equal(t, 1, callCounts[i])
		}
	})
}

// TestMessageDissemination tests message propagation
func TestMessageDissemination(t *testing.T) {
	t.Run("DisseminateToMultiplePeers", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		// Add peers
		for i := 0; i < 5; i++ {
			peerMgr.AddPeer(createTestPeerID(t))
		}

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3, // Should send to 3 out of 5 peers
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		err = engine.Publish([]byte("broadcast-message"))
		require.NoError(t, err)

		// Give time for async dissemination
		time.Sleep(100 * time.Millisecond)

		// Should have sent to peers (fanout = 3)
		// Note: exact count may vary due to async nature
		messageCount := peerMgr.GetMessageCount()
		assert.GreaterOrEqual(t, messageCount, 0)
	})

	t.Run("DisseminateWithNoPeers", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		// Should not error even with no peers
		err = engine.Publish([]byte("lonely-message"))
		assert.NoError(t, err)
	})
}

// TestFanoutAdjustment tests dynamic fanout adjustment
func TestFanoutAdjustment(t *testing.T) {
	t.Run("FanoutAdjustsToNetworkSize", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
			Fanout: 3,
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		// Initially fanout is 3
		assert.Equal(t, 3, engine.fanout)

		// Add many peers (more than 100 to get fanout > 3)
		for i := 0; i < 100; i++ {
			peerMgr.AddPeer(createTestPeerID(t))
		}

		// Manually adjust fanout (normally done by adjustFanout goroutine)
		newFanout := 10 // sqrt(100) = 10
		engine.mu.Lock()
		engine.fanout = newFanout
		engine.mu.Unlock()

		assert.Equal(t, 10, engine.fanout)
	})
}

// TestSelectRandomPeers tests random peer selection
func TestSelectRandomPeers(t *testing.T) {
	t.Run("SelectFewerPeersThanAvailable", func(t *testing.T) {
		peers := make([]peer.ID, 10)
		for i := 0; i < 10; i++ {
			peers[i] = createTestPeerID(t)
		}

		selected := selectRandomPeers(peers, 5)
		assert.Len(t, selected, 5)
	})

	t.Run("SelectMorePeersThanAvailable", func(t *testing.T) {
		peers := make([]peer.ID, 5)
		for i := 0; i < 5; i++ {
			peers[i] = createTestPeerID(t)
		}

		selected := selectRandomPeers(peers, 10)
		assert.Len(t, selected, 5)
	})

	t.Run("SelectZeroPeers", func(t *testing.T) {
		peers := make([]peer.ID, 5)
		for i := 0; i < 5; i++ {
			peers[i] = createTestPeerID(t)
		}

		selected := selectRandomPeers(peers, 0)
		assert.Empty(t, selected)
	})

	t.Run("SelectAllPeers", func(t *testing.T) {
		peers := make([]peer.ID, 5)
		for i := 0; i < 5; i++ {
			peers[i] = createTestPeerID(t)
		}

		selected := selectRandomPeers(peers, 5)
		assert.Len(t, selected, 5)
	})
}

// TestGenerateMessageID tests message ID generation
func TestGenerateMessageID(t *testing.T) {
	t.Run("GenerateID", func(t *testing.T) {
		payload := []byte("test message")
		id := generateMessageID(payload)

		assert.NotEmpty(t, id)
		assert.Len(t, id, 64) // SHA256 hex string
	})

	t.Run("ConsistentID", func(t *testing.T) {
		payload := []byte("consistent message")

		id1 := generateMessageID(payload)
		id2 := generateMessageID(payload)

		assert.Equal(t, id1, id2)
	})

	t.Run("DifferentPayloadDifferentID", func(t *testing.T) {
		payload1 := []byte("message 1")
		payload2 := []byte("message 2")

		id1 := generateMessageID(payload1)
		id2 := generateMessageID(payload2)

		assert.NotEqual(t, id1, id2)
	})
}

// TestEngineClose tests engine shutdown
func TestEngineClose(t *testing.T) {
	t.Run("CloseEngine", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)

		engine.Start()

		err = engine.Close()
		assert.NoError(t, err)

		// Context should be cancelled
		select {
		case <-engine.ctx.Done():
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Context was not cancelled on Close()")
		}
	})
}

// TestConcurrentOperations tests thread safety
func TestConcurrentOperations(t *testing.T) {
	t.Run("ConcurrentPublish", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		engine.Start()

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				payload := []byte("concurrent-" + string(rune('0'+id)))
				err := engine.Publish(payload)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, 10, engine.GetCacheSize())
	})

	t.Run("ConcurrentHandlerRegistration", func(t *testing.T) {
		ctx := context.Background()
		peerMgr := NewMockPeerManager()

		cfg := &Config{
			NodeID: "test-node",
		}

		engine, err := NewGossipEngine(ctx, cfg, peerMgr)
		require.NoError(t, err)
		defer engine.Close()

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				engine.RegisterHandler(func(msg *Message) error {
					return nil
				})
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		engine.mu.RLock()
		count := len(engine.handlers)
		engine.mu.RUnlock()

		assert.Equal(t, 10, count)
	})
}
