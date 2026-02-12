package network

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
)

const (
	// ProtocolID is the protocol identifier for validator communication
	ProtocolID = protocol.ID("/validator/1.0.0")
)

// P2PHost represents a libp2p host for validator nodes
type P2PHost struct {
	host     host.Host
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	peers    map[peer.ID]*PeerConnection
	handlers map[string]MessageHandler
	router   *MessageRouter
}

// PeerConnection represents a connection to a peer
type PeerConnection struct {
	ID            peer.ID
	Addrs         []multiaddr.Multiaddr
	Stream        network.Stream
	LastSeen      int64
	IsValidator   bool
	MessagesSent  int64
	MessagesRecvd int64
}

// MessageHandler is a function that handles incoming messages
type MessageHandler func(peer.ID, []byte) error

// Config holds the configuration for creating a P2P host
type Config struct {
	ListenPort    int
	PrivateKey    crypto.PrivKey
	BootstrapMode bool
	DataDir       string
	NodeID        string // Used for deterministic key generation
}

// GenerateDeterministicKey generates a deterministic Ed25519 private key from a node ID.
// This ensures that the same node ID always produces the same peer ID.
//
// The key seed can be configured via the DTVN_KEY_SEED environment variable.
// When set, it replaces the default salt making keys deployment-specific and unpredictable.
// When unset, the default hardcoded salt is used for development/testing compatibility.
func GenerateDeterministicKey(nodeID string) (crypto.PrivKey, error) {
	// Use a configurable seed so production deployments can set a secret value.
	// The default is only suitable for development/testing.
	salt := os.Getenv("DTVN_KEY_SEED")
	if salt == "" {
		salt = "dtvn-validator-key-v1"
	}
	seed := sha256.Sum256([]byte(salt + nodeID))

	// Generate Ed25519 key from the deterministic seed
	// Ed25519 keys are derived from a 32-byte seed
	privKey, _, err := crypto.GenerateEd25519Key(
		&deterministicReader{seed: seed[:]},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate deterministic key: %w", err)
	}

	return privKey, nil
}

// deterministicReader implements io.Reader using a fixed seed
type deterministicReader struct {
	seed []byte
	pos  int
}

func (r *deterministicReader) Read(p []byte) (n int, err error) {
	// Extend the seed if needed by hashing
	for len(r.seed)-r.pos < len(p) {
		hash := sha256.Sum256(r.seed)
		r.seed = append(r.seed, hash[:]...)
	}
	n = copy(p, r.seed[r.pos:])
	r.pos += n
	return n, nil
}

// NewP2PHost creates a new libp2p host with the specified configuration
func NewP2PHost(ctx context.Context, cfg *Config) (*P2PHost, error) {
	// Generate or load private key
	privKey := cfg.PrivateKey
	if privKey == nil {
		var err error
		// Use deterministic key generation if NodeID is provided
		if cfg.NodeID != "" {
			privKey, err = GenerateDeterministicKey(cfg.NodeID)
			if err != nil {
				return nil, fmt.Errorf("failed to generate deterministic key: %w", err)
			}
		} else {
			// Fallback to random key generation
			privKey, _, err = crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
			if err != nil {
				return nil, fmt.Errorf("failed to generate key pair: %w", err)
			}
		}
	}

	// Create listen address
	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.ListenPort))
	if err != nil {
		return nil, fmt.Errorf("failed to create listen address: %w", err)
	}

	// Create libp2p host with options
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddr),
		// Enable multiple transports
		libp2p.DefaultTransports,
		// Enable multiplexing
		libp2p.DefaultMuxers,
		// Enable security with Noise and TLS
		libp2p.Security(noise.ID, noise.New),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// Enable NAT traversal
		libp2p.NATPortMap(),
		// Enable relay (can relay through other peers)
		libp2p.EnableRelay(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	hostCtx, cancel := context.WithCancel(ctx)

	p2pHost := &P2PHost{
		host:     h,
		ctx:      hostCtx,
		cancel:   cancel,
		peers:    make(map[peer.ID]*PeerConnection),
		handlers: make(map[string]MessageHandler),
	}

	// Set up stream handler
	h.SetStreamHandler(ProtocolID, p2pHost.handleStream)

	// Set up network notification handler to track peer connections
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			p2pHost.mu.Lock()
			if _, exists := p2pHost.peers[peerID]; !exists {
				p2pHost.peers[peerID] = &PeerConnection{
					ID:          peerID,
					Addrs:       []multiaddr.Multiaddr{conn.RemoteMultiaddr()},
					IsValidator: true,
				}
				fmt.Printf("P2P: ✅ Peer connected: %s\n", peerID)
			}
			p2pHost.mu.Unlock()
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			// Only remove if no other connections to this peer
			if len(n.ConnsToPeer(peerID)) == 0 {
				p2pHost.mu.Lock()
				delete(p2pHost.peers, peerID)
				p2pHost.mu.Unlock()
				fmt.Printf("P2P: ❌ Peer disconnected: %s\n", peerID)
			}
		},
	})

	return p2pHost, nil
}

// ID returns the peer ID of this host
func (p *P2PHost) ID() peer.ID {
	return p.host.ID()
}

// Addrs returns the addresses of this host
func (p *P2PHost) Addrs() []multiaddr.Multiaddr {
	return p.host.Addrs()
}

// Host returns the underlying libp2p host
func (p *P2PHost) Host() host.Host {
	return p.host
}

// Connect establishes a connection to a peer
func (p *P2PHost) Connect(ctx context.Context, peerInfo peer.AddrInfo) error {
	if err := p.host.Connect(ctx, peerInfo); err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", peerInfo.ID, err)
	}

	p.mu.Lock()
	p.peers[peerInfo.ID] = &PeerConnection{
		ID:          peerInfo.ID,
		Addrs:       peerInfo.Addrs,
		IsValidator: true,
	}
	p.mu.Unlock()

	return nil
}

// SendMessage sends a message to a specific peer
func (p *P2PHost) SendMessage(ctx context.Context, peerID peer.ID, data []byte) error {
	stream, err := p.host.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return fmt.Errorf("failed to create stream to peer %s: %w", peerID, err)
	}
	defer stream.Close()

	// Write message length prefix (4 bytes)
	msgLen := uint32(len(data))
	lenBuf := make([]byte, 4)
	lenBuf[0] = byte(msgLen >> 24)
	lenBuf[1] = byte(msgLen >> 16)
	lenBuf[2] = byte(msgLen >> 8)
	lenBuf[3] = byte(msgLen)

	if _, err := stream.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// Write message data
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("failed to write message data: %w", err)
	}

	// Update stats
	p.mu.Lock()
	if peerConn, exists := p.peers[peerID]; exists {
		peerConn.MessagesSent++
	}
	p.mu.Unlock()

	return nil
}

// SendTo sends a message to a specific peer by node ID string
func (p *P2PHost) SendTo(ctx context.Context, nodeID string, data []byte) error {
	// Convert string nodeID to peer.ID
	peerID, err := peer.Decode(nodeID)
	if err != nil {
		return fmt.Errorf("failed to decode peer ID %s: %w", nodeID, err)
	}

	return p.SendMessage(ctx, peerID, data)
}

// Broadcast sends a message to all connected peers
func (p *P2PHost) Broadcast(ctx context.Context, data []byte) error {
	p.mu.RLock()
	peers := make([]peer.ID, 0, len(p.peers))
	for peerID := range p.peers {
		peers = append(peers, peerID)
	}
	peerCount := len(peers)
	p.mu.RUnlock()

	// Log broadcast attempt
	fmt.Printf("P2P: Broadcasting message to %d peer(s)\n", peerCount)

	// Warn if no peers (important visibility for debugging)
	if peerCount == 0 {
		fmt.Printf("P2P: ⚠️  WARNING - Broadcasting to ZERO peers, message will not propagate!\n")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(peers))
	successCount := 0
	var successMu sync.Mutex

	for _, peerID := range peers {
		wg.Add(1)
		go func(pid peer.ID) {
			defer wg.Done()
			if err := p.SendMessage(ctx, pid, data); err != nil {
				fmt.Printf("P2P: Failed to send to peer %s: %v\n", pid, err)
				errChan <- err
			} else {
				successMu.Lock()
				successCount++
				successMu.Unlock()
			}
		}(peerID)
	}

	wg.Wait()
	close(errChan)

	// Log success summary
	if peerCount > 0 {
		fmt.Printf("P2P: ✅ Successfully broadcast to %d/%d peer(s)\n", successCount, peerCount)
	}

	// Return first error if any
	for err := range errChan {
		return err
	}

	return nil
}

// RegisterHandler registers a message handler for a specific message type
func (p *P2PHost) RegisterHandler(messageType string, handler MessageHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[messageType] = handler
}

// SetRouter sets the message router for this host
func (p *P2PHost) SetRouter(router *MessageRouter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.router = router
}

// handleStream handles incoming streams from peers
func (p *P2PHost) handleStream(stream network.Stream) {
	defer stream.Close()

	peerID := stream.Conn().RemotePeer()

	// Read message length - use io.ReadFull to ensure we read all 4 bytes
	// Note: stream.Read() may return fewer bytes than requested, causing
	// corrupted message length parsing and subsequent message routing failures
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		fmt.Printf("Error reading message length from peer %s: %v\n", peerID, err)
		return
	}

	msgLen := uint32(lenBuf[0])<<24 | uint32(lenBuf[1])<<16 | uint32(lenBuf[2])<<8 | uint32(lenBuf[3])

	// Sanity check message length to prevent memory exhaustion
	const maxMsgLen = 10 * 1024 * 1024 // 10MB max
	if msgLen > maxMsgLen {
		fmt.Printf("Error: message length %d exceeds maximum %d from peer %s\n", msgLen, maxMsgLen, peerID)
		return
	}

	// Read message data - use io.ReadFull to ensure we read the complete message
	// This is critical: partial reads cause message deserialization failures
	// which break PBFT consensus (PREPARE messages never reach the handler)
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, data); err != nil {
		fmt.Printf("Error reading message data from peer %s: %v\n", peerID, err)
		return
	}

	// Update stats
	p.mu.Lock()
	if peerConn, exists := p.peers[peerID]; exists {
		peerConn.MessagesRecvd++
	} else {
		p.peers[peerID] = &PeerConnection{
			ID:            peerID,
			MessagesRecvd: 1,
		}
	}
	p.mu.Unlock()

	// Route message through the router if available
	p.mu.RLock()
	router := p.router
	p.mu.RUnlock()

	if router != nil {
		// Use the new message router
		if err := router.RouteMessage(data, peerID); err != nil {
			fmt.Printf("Error routing message from peer %s: %v\n", peerID, err)
		}
	} else {
		// Fallback to legacy handlers if router not set
		p.mu.RLock()
		handlers := make(map[string]MessageHandler)
		for k, v := range p.handlers {
			handlers[k] = v
		}
		p.mu.RUnlock()

		for _, handler := range handlers {
			if err := handler(peerID, data); err != nil {
				fmt.Printf("Error handling message from peer %s: %v\n", peerID, err)
			}
		}
	}
}

// GetPeers returns a list of connected peers
func (p *P2PHost) GetPeers() []peer.ID {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peers := make([]peer.ID, 0, len(p.peers))
	for peerID := range p.peers {
		peers = append(peers, peerID)
	}
	return peers
}

// GetConnectedPeers returns detailed information about connected peers
func (p *P2PHost) GetConnectedPeers() []peer.AddrInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peerInfos := make([]peer.AddrInfo, 0, len(p.peers))
	for peerID, peerConn := range p.peers {
		peerInfos = append(peerInfos, peer.AddrInfo{
			ID:    peerID,
			Addrs: peerConn.Addrs,
		})
	}
	return peerInfos
}

// GetPeerCount returns the number of connected peers
func (p *P2PHost) GetPeerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.peers)
}

// Close shuts down the host and all connections
func (p *P2PHost) Close() error {
	p.cancel()
	return p.host.Close()
}
