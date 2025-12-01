package network

import (
	"context"
	"crypto/rand"
	"fmt"
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
}

// NewP2PHost creates a new libp2p host with the specified configuration
func NewP2PHost(ctx context.Context, cfg *Config) (*P2PHost, error) {
	// Generate or load private key
	privKey := cfg.PrivateKey
	if privKey == nil {
		var err error
		privKey, _, err = crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate key pair: %w", err)
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
		// Enable relay
		libp2p.EnableRelay(),
		// Enable auto relay if not bootstrap node
		libp2p.EnableAutoRelay(),
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

// Broadcast sends a message to all connected peers
func (p *P2PHost) Broadcast(ctx context.Context, data []byte) error {
	p.mu.RLock()
	peers := make([]peer.ID, 0, len(p.peers))
	for peerID := range p.peers {
		peers = append(peers, peerID)
	}
	p.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(peers))

	for _, peerID := range peers {
		wg.Add(1)
		go func(pid peer.ID) {
			defer wg.Done()
			if err := p.SendMessage(ctx, pid, data); err != nil {
				errChan <- err
			}
		}(peerID)
	}

	wg.Wait()
	close(errChan)

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

// handleStream handles incoming streams from peers
func (p *P2PHost) handleStream(stream network.Stream) {
	defer stream.Close()

	peerID := stream.Conn().RemotePeer()

	// Read message length
	lenBuf := make([]byte, 4)
	if _, err := stream.Read(lenBuf); err != nil {
		fmt.Printf("Error reading message length from peer %s: %v\n", peerID, err)
		return
	}

	msgLen := uint32(lenBuf[0])<<24 | uint32(lenBuf[1])<<16 | uint32(lenBuf[2])<<8 | uint32(lenBuf[3])

	// Read message data
	data := make([]byte, msgLen)
	if _, err := stream.Read(data); err != nil {
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

	// Call registered handlers
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
