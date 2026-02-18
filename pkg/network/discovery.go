package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
)

const (
	// ValidatorRendezvous is the namespace for validator node discovery
	ValidatorRendezvous = "validator-network"
	// DiscoveryInterval is how often we re-advertise and discover peers
	DiscoveryInterval = 10 * time.Second
	// MaxConcurrentConnections limits simultaneous outgoing peer connection attempts
	// during discovery to prevent connection storms.
	MaxConcurrentConnections = 5
)

// Discovery handles peer discovery using Kademlia DHT
type Discovery struct {
	host          host.Host
	dht           *dht.IpfsDHT
	routingDiscov *routing.RoutingDiscovery
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.RWMutex
	peers         map[peer.ID]peer.AddrInfo
	onPeerFound   func(peer.AddrInfo)
}

// DiscoveryConfig holds configuration for peer discovery
type DiscoveryConfig struct {
	BootstrapPeers []peer.AddrInfo
	IsBootstrap    bool
}

// NewDiscovery creates a new DHT-based peer discovery service
func NewDiscovery(ctx context.Context, h host.Host, cfg *DiscoveryConfig) (*Discovery, error) {
	// All nodes run in server mode so they can be discovered by peers via DHT.
	// Client mode prevents nodes from appearing in DHT routing, which breaks
	// peer discovery between non-bootstrap nodes.
	dhtOpts := []dht.Option{dht.Mode(dht.ModeServer)}

	// Create the DHT
	kdht, err := dht.New(ctx, h, dhtOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}

	// Bootstrap the DHT
	if err = kdht.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	// Connect to bootstrap peers if any
	if len(cfg.BootstrapPeers) > 0 {
		if err := connectToBootstrapPeers(ctx, h, cfg.BootstrapPeers); err != nil {
			return nil, fmt.Errorf("failed to connect to bootstrap peers: %w", err)
		}
	}

	// Create routing discovery
	routingDiscov := routing.NewRoutingDiscovery(kdht)

	discoveryCtx, cancel := context.WithCancel(ctx)

	d := &Discovery{
		host:          h,
		dht:           kdht,
		routingDiscov: routingDiscov,
		ctx:           discoveryCtx,
		cancel:        cancel,
		peers:         make(map[peer.ID]peer.AddrInfo),
	}

	return d, nil
}

// Start begins the discovery process
func (d *Discovery) Start() error {
	// Announce ourselves as a validator
	util.Advertise(d.ctx, d.routingDiscov, ValidatorRendezvous)

	// Run initial discovery immediately (don't wait 30s)
	fmt.Printf("Discovery: Running initial peer discovery...\n")
	go d.discoverPeers()

	// Run again after short delays to catch newly started nodes.
	time.AfterFunc(3*time.Second, func() {
		fmt.Printf("Discovery: Running second discovery pass...\n")
		d.discoverPeers()
	})
	time.AfterFunc(6*time.Second, func() {
		fmt.Printf("Discovery: Running third discovery pass...\n")
		d.discoverPeers()
	})
	time.AfterFunc(10*time.Second, func() {
		fmt.Printf("Discovery: Running fourth discovery pass...\n")
		d.discoverPeers()
	})

	// Start periodic discovery loop
	go d.discoveryLoop()

	return nil
}

// discoveryLoop continuously discovers new peers
func (d *Discovery) discoveryLoop() {
	ticker := time.NewTicker(DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.discoverPeers()
		}
	}
}

// discoverPeers finds and connects to validator peers
func (d *Discovery) discoverPeers() {
	ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	defer cancel()

	// Re-advertise ourselves
	util.Advertise(ctx, d.routingDiscov, ValidatorRendezvous)

	// Find peers
	peerChan, err := d.routingDiscov.FindPeers(ctx, ValidatorRendezvous)
	if err != nil {
		fmt.Printf("Error finding peers: %v\n", err)
		return
	}

	// Semaphore to limit concurrent outgoing connection attempts
	sem := make(chan struct{}, MaxConcurrentConnections)

	// Process discovered peers
	for peerInfo := range peerChan {
		// Skip ourselves
		if peerInfo.ID == d.host.ID() {
			continue
		}

		// Skip if already connected
		if d.host.Network().Connectedness(peerInfo.ID) == 1 { // Connected
			continue
		}

		// Store peer info
		d.mu.Lock()
		d.peers[peerInfo.ID] = peerInfo
		d.mu.Unlock()

		// Notify about new peer
		if d.onPeerFound != nil {
			d.onPeerFound(peerInfo)
		}

		// Try to connect (bounded concurrency)
		sem <- struct{}{} // Acquire
		go func(pi peer.AddrInfo) {
			defer func() { <-sem }() // Release

			connectCtx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
			defer cancel()

			if err := d.host.Connect(connectCtx, pi); err != nil {
				fmt.Printf("Failed to connect to peer %s: %v\n", pi.ID, err)
			} else {
				fmt.Printf("Connected to peer %s\n", pi.ID)
			}
		}(peerInfo)
	}
}

// OnPeerFound sets a callback for when new peers are discovered
func (d *Discovery) OnPeerFound(callback func(peer.AddrInfo)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onPeerFound = callback
}

// GetPeers returns all discovered peers
func (d *Discovery) GetPeers() []peer.AddrInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	peers := make([]peer.AddrInfo, 0, len(d.peers))
	for _, peerInfo := range d.peers {
		peers = append(peers, peerInfo)
	}
	return peers
}

// FindPeer looks up a specific peer in the DHT
func (d *Discovery) FindPeer(ctx context.Context, peerID peer.ID) (peer.AddrInfo, error) {
	return d.dht.FindPeer(ctx, peerID)
}

// GetRoutingTable returns information about the DHT routing table
func (d *Discovery) GetRoutingTable() []peer.ID {
	return d.dht.RoutingTable().ListPeers()
}

// Close shuts down the discovery service
func (d *Discovery) Close() error {
	d.cancel()
	return d.dht.Close()
}

// connectToBootstrapPeers connects to the initial bootstrap peers
func connectToBootstrapPeers(ctx context.Context, h host.Host, bootstrapPeers []peer.AddrInfo) error {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	for _, peerInfo := range bootstrapPeers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()

			connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			// If peer ID is empty, skip - we can't connect without it
			if pi.ID == "" {
				fmt.Printf("Discovery: ⚠️  Skipping bootstrap peer without peer ID. Address: %v\n", pi.Addrs)
				fmt.Printf("Discovery: To connect, use full multiaddr format: /ip4/x.x.x.x/tcp/port/p2p/<peer-id>\n")
				fmt.Printf("Discovery: Peer will be discovered via DHT instead.\n")
				// Don't treat this as an error - DHT discovery will find peers
				return
			}

			// Normal case: we have peer ID
			if err := h.Connect(connectCtx, pi); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to connect to bootstrap peer %s: %w", pi.ID, err)
				}
				mu.Unlock()
				return
			}

			fmt.Printf("Discovery: ✅ Connected to bootstrap peer %s\n", pi.ID)
		}(peerInfo)
	}

	wg.Wait()
	return firstErr
}
