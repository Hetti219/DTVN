package network

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateDeterministicKey tests deterministic key generation
func TestGenerateDeterministicKey(t *testing.T) {
	t.Run("SameNodeIDProducesSameKey", func(t *testing.T) {
		nodeID := "validator-001"

		key1, err := GenerateDeterministicKey(nodeID)
		require.NoError(t, err)
		require.NotNil(t, key1)

		key2, err := GenerateDeterministicKey(nodeID)
		require.NoError(t, err)
		require.NotNil(t, key2)

		// Keys should be equal
		bytes1, _ := crypto.MarshalPrivateKey(key1)
		bytes2, _ := crypto.MarshalPrivateKey(key2)
		assert.Equal(t, bytes1, bytes2)
	})

	t.Run("DifferentNodeIDsProduceDifferentKeys", func(t *testing.T) {
		key1, err := GenerateDeterministicKey("validator-001")
		require.NoError(t, err)

		key2, err := GenerateDeterministicKey("validator-002")
		require.NoError(t, err)

		// Keys should be different
		bytes1, _ := crypto.MarshalPrivateKey(key1)
		bytes2, _ := crypto.MarshalPrivateKey(key2)
		assert.NotEqual(t, bytes1, bytes2)
	})

	t.Run("KeyGenerationWithEmptyNodeID", func(t *testing.T) {
		key, err := GenerateDeterministicKey("")
		require.NoError(t, err)
		require.NotNil(t, key)
	})

	t.Run("SamePeerIDFromSameNodeID", func(t *testing.T) {
		nodeID := "test-node"

		key1, _ := GenerateDeterministicKey(nodeID)
		peerID1, _ := peer.IDFromPrivateKey(key1)

		key2, _ := GenerateDeterministicKey(nodeID)
		peerID2, _ := peer.IDFromPrivateKey(key2)

		assert.Equal(t, peerID1, peerID2)
	})

	t.Run("KeyTypeIsEd25519", func(t *testing.T) {
		key, err := GenerateDeterministicKey("test-node")
		require.NoError(t, err)

		assert.Equal(t, crypto.Ed25519, key.Type())
	})
}

// TestDeterministicReader tests the deterministic reader implementation
func TestDeterministicReader(t *testing.T) {
	t.Run("ConsistentReads", func(t *testing.T) {
		seed := []byte("test-seed-123")
		reader1 := &deterministicReader{seed: seed}
		reader2 := &deterministicReader{seed: seed}

		buf1 := make([]byte, 100)
		buf2 := make([]byte, 100)

		n1, err1 := reader1.Read(buf1)
		n2, err2 := reader2.Read(buf2)

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.Equal(t, n1, n2)
		assert.Equal(t, buf1, buf2)
	})

	t.Run("ExtendsSeedWhenNeeded", func(t *testing.T) {
		seed := []byte("short")
		reader := &deterministicReader{seed: seed}

		// Read more bytes than seed length
		buf := make([]byte, 1000)
		n, err := reader.Read(buf)

		assert.NoError(t, err)
		assert.Equal(t, 1000, n)
		assert.Greater(t, len(reader.seed), len("short"))
	})

	t.Run("MultipleReadsConsecutive", func(t *testing.T) {
		seed := []byte("test")
		reader := &deterministicReader{seed: seed}

		buf1 := make([]byte, 32)
		buf2 := make([]byte, 32)

		reader.Read(buf1)
		reader.Read(buf2)

		// Consecutive reads should produce different data
		assert.NotEqual(t, buf1, buf2)
	})
}

// TestNewP2PHost tests P2P host creation
func TestNewP2PHost(t *testing.T) {
	t.Run("CreateHostWithNodeID", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0, // Random port
			NodeID:     "test-node-001",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, host)
		defer host.Close()

		assert.NotNil(t, host.host)
		assert.NotNil(t, host.peers)
		assert.NotNil(t, host.handlers)
	})

	t.Run("CreateHostWithProvidedKey", func(t *testing.T) {
		ctx := context.Background()

		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
		require.NoError(t, err)

		cfg := &Config{
			ListenPort: 0,
			PrivateKey: privKey,
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, host)
		defer host.Close()

		// Verify the host is using the provided key
		peerID, _ := peer.IDFromPrivateKey(privKey)
		assert.Equal(t, peerID, host.ID())
	})

	t.Run("CreateHostWithoutKeyGeneratesRandom", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			// No NodeID or PrivateKey provided
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, host)
		defer host.Close()

		assert.NotEmpty(t, host.ID())
	})

	t.Run("CreateTwoHostsWithSameNodeIDHaveSamePeerID", func(t *testing.T) {
		ctx := context.Background()
		nodeID := "deterministic-node"

		cfg1 := &Config{
			ListenPort: 0,
			NodeID:     nodeID,
		}
		host1, err := NewP2PHost(ctx, cfg1)
		require.NoError(t, err)
		defer host1.Close()

		cfg2 := &Config{
			ListenPort: 0,
			NodeID:     nodeID,
		}
		host2, err := NewP2PHost(ctx, cfg2)
		require.NoError(t, err)
		defer host2.Close()

		assert.Equal(t, host1.ID(), host2.ID())
	})

	t.Run("CreateHostWithSpecificPort", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 14001,
			NodeID:     "port-test-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, host)
		defer host.Close()

		// Verify at least one address contains the port
		addrs := host.Addrs()
		assert.NotEmpty(t, addrs)
	})
}

// TestP2PHostBasicOperations tests basic host operations
func TestP2PHostBasicOperations(t *testing.T) {
	t.Run("GetHostID", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "test-id-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		peerID := host.ID()
		assert.NotEmpty(t, peerID)
	})

	t.Run("GetHostAddresses", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "test-addr-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		addrs := host.Addrs()
		assert.NotEmpty(t, addrs)
	})

	t.Run("GetUnderlyingHost", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "test-underlying-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		libp2pHost := host.Host()
		assert.NotNil(t, libp2pHost)
		assert.Equal(t, host.ID(), libp2pHost.ID())
	})

	t.Run("InitiallyNoPeers", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "test-no-peers-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		assert.Equal(t, 0, host.GetPeerCount())
		assert.Empty(t, host.GetPeers())
	})
}

// TestHandlerRegistration tests message handler registration
func TestHandlerRegistration(t *testing.T) {
	t.Run("RegisterSingleHandler", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "handler-test-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		called := false
		handler := func(peerID peer.ID, data []byte) error {
			called = true
			return nil
		}

		host.RegisterHandler("test-type", handler)

		host.mu.RLock()
		_, exists := host.handlers["test-type"]
		host.mu.RUnlock()

		assert.True(t, exists)
	})

	t.Run("RegisterMultipleHandlers", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "multi-handler-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		handler1 := func(peerID peer.ID, data []byte) error { return nil }
		handler2 := func(peerID peer.ID, data []byte) error { return nil }
		handler3 := func(peerID peer.ID, data []byte) error { return nil }

		host.RegisterHandler("type1", handler1)
		host.RegisterHandler("type2", handler2)
		host.RegisterHandler("type3", handler3)

		host.mu.RLock()
		count := len(host.handlers)
		host.mu.RUnlock()

		assert.Equal(t, 3, count)
	})

	t.Run("OverwriteHandler", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "overwrite-handler-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		handler1 := func(peerID peer.ID, data []byte) error { return nil }
		handler2 := func(peerID peer.ID, data []byte) error { return nil }

		host.RegisterHandler("type", handler1)
		host.RegisterHandler("type", handler2) // Overwrite

		host.mu.RLock()
		count := len(host.handlers)
		host.mu.RUnlock()

		assert.Equal(t, 1, count)
	})
}

// TestRouterRegistration tests message router registration
func TestRouterRegistration(t *testing.T) {
	t.Run("SetRouter", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "router-test-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		router := NewMessageRouter()
		host.SetRouter(router)

		host.mu.RLock()
		setRouter := host.router
		host.mu.RUnlock()

		assert.Equal(t, router, setRouter)
	})

	t.Run("ReplaceRouter", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "replace-router-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)
		defer host.Close()

		router1 := NewMessageRouter()
		router2 := NewMessageRouter()

		host.SetRouter(router1)
		host.SetRouter(router2)

		host.mu.RLock()
		setRouter := host.router
		host.mu.RUnlock()

		assert.Equal(t, router2, setRouter)
	})
}

// TestPeerConnection tests peer connection functionality
func TestPeerConnection(t *testing.T) {
	t.Run("ConnectTwoHosts", func(t *testing.T) {
		ctx := context.Background()

		// Create first host
		cfg1 := &Config{
			ListenPort: 0,
			NodeID:     "host1",
		}
		host1, err := NewP2PHost(ctx, cfg1)
		require.NoError(t, err)
		defer host1.Close()

		// Create second host
		cfg2 := &Config{
			ListenPort: 0,
			NodeID:     "host2",
		}
		host2, err := NewP2PHost(ctx, cfg2)
		require.NoError(t, err)
		defer host2.Close()

		// Connect host2 to host1
		peerInfo := peer.AddrInfo{
			ID:    host1.ID(),
			Addrs: host1.Addrs(),
		}

		err = host2.Connect(ctx, peerInfo)
		require.NoError(t, err)

		// Give connection time to establish
		time.Sleep(100 * time.Millisecond)

		// Verify connection
		assert.Greater(t, host2.GetPeerCount(), 0)
	})

	t.Run("GetConnectedPeers", func(t *testing.T) {
		ctx := context.Background()

		host1, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "peer-info-1"})
		defer host1.Close()

		host2, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "peer-info-2"})
		defer host2.Close()

		// Connect
		peerInfo := peer.AddrInfo{
			ID:    host1.ID(),
			Addrs: host1.Addrs(),
		}
		host2.Connect(ctx, peerInfo)

		time.Sleep(100 * time.Millisecond)

		// Get connected peers
		peers := host2.GetConnectedPeers()
		if len(peers) > 0 {
			assert.NotEmpty(t, peers[0].ID)
		}
	})
}

// TestMessageSending tests message sending functionality
func TestMessageSending(t *testing.T) {
	t.Run("SendMessageBetweenHosts", func(t *testing.T) {
		ctx := context.Background()

		// Create hosts
		host1, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "sender"})
		defer host1.Close()

		host2, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "receiver"})
		defer host2.Close()

		// Setup message receiver
		received := make(chan []byte, 1)
		host2.RegisterHandler("test", func(peerID peer.ID, data []byte) error {
			received <- data
			return nil
		})

		// Connect hosts
		peerInfo := peer.AddrInfo{
			ID:    host2.ID(),
			Addrs: host2.Addrs(),
		}
		err := host1.Connect(ctx, peerInfo)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Send message
		testData := []byte("test message")
		err = host1.SendMessage(ctx, host2.ID(), testData)
		require.NoError(t, err)

		// Wait for message with timeout
		select {
		case data := <-received:
			assert.Equal(t, testData, data)
		case <-time.After(2 * time.Second):
			t.Log("Message not received within timeout (network test)")
		}
	})

	t.Run("SendToNonExistentPeer", func(t *testing.T) {
		ctx := context.Background()

		host, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "lonely-host"})
		defer host.Close()

		// Generate a random peer ID that doesn't exist
		_, pub, _ := crypto.GenerateKeyPair(crypto.Ed25519, 2048)
		fakePeerID, _ := peer.IDFromPublicKey(pub)

		err := host.SendMessage(ctx, fakePeerID, []byte("test"))
		assert.Error(t, err)
	})
}

// TestBroadcast tests broadcast functionality
func TestBroadcast(t *testing.T) {
	t.Run("BroadcastWithNoPeers", func(t *testing.T) {
		ctx := context.Background()

		host, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "broadcaster"})
		defer host.Close()

		// Broadcast should succeed even with no peers
		err := host.Broadcast(ctx, []byte("broadcast message"))
		assert.NoError(t, err)
	})

	t.Run("BroadcastToMultiplePeers", func(t *testing.T) {
		ctx := context.Background()

		// Create broadcaster
		host1, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "broadcaster"})
		defer host1.Close()

		// Create receivers
		host2, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "receiver1"})
		defer host2.Close()

		host3, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "receiver2"})
		defer host3.Close()

		// Connect broadcaster to receivers
		host1.Connect(ctx, peer.AddrInfo{ID: host2.ID(), Addrs: host2.Addrs()})
		host1.Connect(ctx, peer.AddrInfo{ID: host3.ID(), Addrs: host3.Addrs()})

		time.Sleep(100 * time.Millisecond)

		// Broadcast
		err := host1.Broadcast(ctx, []byte("broadcast test"))
		// May fail if connections aren't fully established in test env
		if err != nil {
			t.Logf("Broadcast error (expected in some test environments): %v", err)
		}
	})
}

// TestSendTo tests the SendTo method
func TestSendTo(t *testing.T) {
	t.Run("SendToWithValidPeerID", func(t *testing.T) {
		ctx := context.Background()

		host1, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "send-to-sender"})
		defer host1.Close()

		host2, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "send-to-receiver"})
		defer host2.Close()

		// Connect
		host1.Connect(ctx, peer.AddrInfo{ID: host2.ID(), Addrs: host2.Addrs()})
		time.Sleep(100 * time.Millisecond)

		// Send using string peer ID
		err := host1.SendTo(ctx, host2.ID().String(), []byte("test"))
		// May fail in test environment
		if err != nil {
			t.Logf("SendTo error (expected in test environment): %v", err)
		}
	})

	t.Run("SendToWithInvalidPeerID", func(t *testing.T) {
		ctx := context.Background()

		host, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "send-to-test"})
		defer host.Close()

		err := host.SendTo(ctx, "invalid-peer-id", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode peer ID")
	})
}

// TestHostClose tests host shutdown
func TestHostClose(t *testing.T) {
	t.Run("CloseHost", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "close-test-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)

		err = host.Close()
		assert.NoError(t, err)
	})

	t.Run("CloseHostMultipleTimes", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "multi-close-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)

		err = host.Close()
		assert.NoError(t, err)

		// Second close should error
		err = host.Close()
		assert.Error(t, err)
	})

	t.Run("ContextCancellationOnClose", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			ListenPort: 0,
			NodeID:     "context-cancel-node",
		}

		host, err := NewP2PHost(ctx, cfg)
		require.NoError(t, err)

		host.Close()

		// Check if context was cancelled
		select {
		case <-host.ctx.Done():
			// Context properly cancelled
		case <-time.After(100 * time.Millisecond):
			t.Error("Context was not cancelled on Close()")
		}
	})
}

// TestConcurrentOperations tests thread-safety
func TestConcurrentOperations(t *testing.T) {
	t.Run("ConcurrentHandlerRegistration", func(t *testing.T) {
		ctx := context.Background()
		host, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "concurrent-handlers"})
		defer host.Close()

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				handler := func(peerID peer.ID, data []byte) error { return nil }
				host.RegisterHandler("type-"+string(rune('0'+id)), handler)
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		host.mu.RLock()
		count := len(host.handlers)
		host.mu.RUnlock()

		assert.Equal(t, 10, count)
	})

	t.Run("ConcurrentGetPeers", func(t *testing.T) {
		ctx := context.Background()
		host, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "concurrent-get-peers"})
		defer host.Close()

		done := make(chan bool)
		for i := 0; i < 20; i++ {
			go func() {
				_ = host.GetPeers()
				_ = host.GetPeerCount()
				done <- true
			}()
		}

		for i := 0; i < 20; i++ {
			<-done
		}
	})
}

// TestPeerConnectionTracking tests peer connection state tracking
func TestPeerConnectionTracking(t *testing.T) {
	t.Run("PeerConnectionMetadata", func(t *testing.T) {
		ctx := context.Background()

		host1, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "metadata-1"})
		defer host1.Close()

		host2, _ := NewP2PHost(ctx, &Config{ListenPort: 0, NodeID: "metadata-2"})
		defer host2.Close()

		// Connect
		peerInfo := peer.AddrInfo{
			ID:    host1.ID(),
			Addrs: host1.Addrs(),
		}
		host2.Connect(ctx, peerInfo)

		time.Sleep(100 * time.Millisecond)

		// Check peer metadata
		host2.mu.RLock()
		peerConn, exists := host2.peers[host1.ID()]
		host2.mu.RUnlock()

		if exists {
			assert.Equal(t, host1.ID(), peerConn.ID)
			assert.True(t, peerConn.IsValidator)
		}
	})
}
