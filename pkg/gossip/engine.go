package gossip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// DefaultTTL is the default time-to-live for gossip messages
	DefaultTTL = 10
	// AntiEntropyInterval is how often anti-entropy runs
	AntiEntropyInterval = 5 * time.Second
	// MessageCacheSize is the size of the message cache
	MessageCacheSize = 10000
	// BloomFilterFPRate is the false positive rate for bloom filters
	BloomFilterFPRate = 0.01
	// SentMessagesTTL is how long sent message IDs are retained before cleanup
	SentMessagesTTL = 10 * time.Minute
	// SentMessagesCleanupInterval is how often old sent message entries are evicted
	SentMessagesCleanupInterval = 2 * time.Minute
	// SentMessagesMaxSize is the hard cap on sent messages map entries.
	// If the map exceeds this after a cleanup pass, the oldest entries are evicted.
	SentMessagesMaxSize = 50000
)

// Message represents a gossip message
type Message struct {
	ID        string
	Payload   []byte
	TTL       int32
	SeenBy    []string
	Timestamp int64
}

// GossipEngine handles epidemic broadcast of messages
type GossipEngine struct {
	nodeID          string
	fanout          int
	cache           *MessageCache
	peers           PeerManager
	messageChan     chan *Message
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.RWMutex
	handlers        []MessageHandler
	sentMessages    map[string]time.Time // msgID -> insertion time (bounded with periodic cleanup)
	receivedDigests map[string]time.Time // digest -> insertion time (bounded with periodic cleanup)
}

// MessageHandler is called when a new message is received
type MessageHandler func(*Message) error

// PeerManager interface for managing peers
type PeerManager interface {
	GetPeers() []peer.ID
	GetPeerCount() int
	SendMessage(ctx context.Context, peerID peer.ID, data []byte) error
}

// Config holds gossip engine configuration
type Config struct {
	NodeID string
	Fanout int // Number of peers to gossip to (default: sqrt(n))
}

// NewGossipEngine creates a new gossip engine
func NewGossipEngine(ctx context.Context, cfg *Config, peerMgr PeerManager) (*GossipEngine, error) {
	engineCtx, cancel := context.WithCancel(ctx)

	fanout := cfg.Fanout
	if fanout == 0 {
		// Default fanout is sqrt(n), start with 3
		fanout = 3
	}

	engine := &GossipEngine{
		nodeID:          cfg.NodeID,
		fanout:          fanout,
		cache:           NewMessageCache(MessageCacheSize),
		peers:           peerMgr,
		messageChan:     make(chan *Message, 1000),
		ctx:             engineCtx,
		cancel:          cancel,
		handlers:        make([]MessageHandler, 0),
		sentMessages:    make(map[string]time.Time),
		receivedDigests: make(map[string]time.Time),
	}

	return engine, nil
}

// Start begins the gossip engine
func (g *GossipEngine) Start() {
	// Start message processing goroutine
	go g.processMessages()

	// Start anti-entropy goroutine
	go g.antiEntropyLoop()

	// Start fanout adjuster
	go g.adjustFanout()

	// Start periodic cleanup of bounded maps
	go g.cleanupLoop()
}

// Publish publishes a new message to the network
func (g *GossipEngine) Publish(payload []byte) error {
	msg := &Message{
		ID:        generateMessageID(payload),
		Payload:   payload,
		TTL:       DefaultTTL,
		SeenBy:    []string{g.nodeID},
		Timestamp: time.Now().Unix(),
	}

	// Add to cache
	g.cache.Add(msg)

	// Add to sent messages
	g.mu.Lock()
	g.sentMessages[msg.ID] = time.Now()
	g.mu.Unlock()

	// Send to message channel
	select {
	case g.messageChan <- msg:
	case <-g.ctx.Done():
		return g.ctx.Err()
	}

	return nil
}

// ReceiveMessage handles incoming gossip messages
func (g *GossipEngine) ReceiveMessage(msg *Message) error {
	// Check if we've seen this message
	if g.cache.Contains(msg.ID) {
		return nil // Already processed
	}

	// Add to cache
	g.cache.Add(msg)

	// Add ourselves to seen list
	msg.SeenBy = append(msg.SeenBy, g.nodeID)

	// Decrement TTL
	msg.TTL--

	// Call handlers
	g.mu.RLock()
	handlers := g.handlers
	g.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(msg); err != nil {
			fmt.Printf("Handler error: %v\n", err)
		}
	}

	// Propagate if TTL > 0
	if msg.TTL > 0 {
		select {
		case g.messageChan <- msg:
		case <-g.ctx.Done():
			return g.ctx.Err()
		}
	}

	return nil
}

// RegisterHandler registers a message handler
func (g *GossipEngine) RegisterHandler(handler MessageHandler) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.handlers = append(g.handlers, handler)
}

// processMessages processes messages from the queue
func (g *GossipEngine) processMessages() {
	for {
		select {
		case <-g.ctx.Done():
			return
		case msg := <-g.messageChan:
			g.disseminateMessage(msg)
		}
	}
}

// disseminateMessage sends a message to random peers (push gossip)
func (g *GossipEngine) disseminateMessage(msg *Message) {
	peers := g.peers.GetPeers()
	if len(peers) == 0 {
		return
	}

	// Calculate fanout (min of configured fanout and available peers)
	fanout := g.fanout
	if fanout > len(peers) {
		fanout = len(peers)
	}

	// Select random peers
	selectedPeers := selectRandomPeers(peers, fanout)

	// Send to selected peers
	for _, peerID := range selectedPeers {
		// Serialize message (in production, use protobuf)
		data := serializeMessage(msg)

		go func(pid peer.ID) {
			ctx, cancel := context.WithTimeout(g.ctx, 5*time.Second)
			defer cancel()

			if err := g.peers.SendMessage(ctx, pid, data); err != nil {
				fmt.Printf("Failed to send message to peer %s: %v\n", pid, err)
			}
		}(peerID)
	}
}

// antiEntropyLoop periodically reconciles with random peers
func (g *GossipEngine) antiEntropyLoop() {
	ticker := time.NewTicker(AntiEntropyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.performAntiEntropy()
		}
	}
}

// performAntiEntropy performs anti-entropy with a random peer
func (g *GossipEngine) performAntiEntropy() {
	peers := g.peers.GetPeers()
	if len(peers) == 0 {
		return
	}

	// Select a random peer
	randomPeer := peers[rand.Intn(len(peers))]

	// Get message digests from cache
	digests := g.cache.GetDigests()

	// Exchange digests
	// request missing messages from the peer
	ctx, cancel := context.WithTimeout(g.ctx, 10*time.Second)
	defer cancel()

	// Serialize digests
	data := serializeDigests(digests)

	if err := g.peers.SendMessage(ctx, randomPeer, data); err != nil {
		fmt.Printf("Anti-entropy failed with peer %s: %v\n", randomPeer, err)
	}
}

// adjustFanout dynamically adjusts fanout based on network size
func (g *GossipEngine) adjustFanout() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			peerCount := g.peers.GetPeerCount()
			// Fanout = sqrt(n)
			newFanout := int(math.Sqrt(float64(peerCount)))
			if newFanout < 3 {
				newFanout = 3 // Minimum fanout
			}
			if newFanout > 10 {
				newFanout = 10 // Maximum fanout
			}

			g.mu.Lock()
			g.fanout = newFanout
			g.mu.Unlock()
		}
	}
}

// cleanupLoop periodically evicts expired entries from sentMessages and receivedDigests
// to prevent unbounded memory growth.
func (g *GossipEngine) cleanupLoop() {
	ticker := time.NewTicker(SentMessagesCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.cleanupExpiredEntries()
		}
	}
}

// cleanupExpiredEntries removes entries older than SentMessagesTTL from both maps.
// If a map still exceeds SentMessagesMaxSize after TTL eviction, the oldest entries
// are dropped until the map is within bounds.
func (g *GossipEngine) cleanupExpiredEntries() {
	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-SentMessagesTTL)

	// Evict expired sentMessages
	for id, ts := range g.sentMessages {
		if ts.Before(cutoff) {
			delete(g.sentMessages, id)
		}
	}

	// Hard cap: if still over limit, drop oldest entries
	if len(g.sentMessages) > SentMessagesMaxSize {
		// Find the oldest entries to remove
		excess := len(g.sentMessages) - SentMessagesMaxSize
		type entry struct {
			id string
			ts time.Time
		}
		oldest := make([]entry, 0, excess)
		for id, ts := range g.sentMessages {
			if len(oldest) < excess {
				oldest = append(oldest, entry{id, ts})
			} else {
				// Replace the newest in oldest[] if this entry is older
				maxIdx := 0
				for i := 1; i < len(oldest); i++ {
					if oldest[i].ts.After(oldest[maxIdx].ts) {
						maxIdx = i
					}
				}
				if ts.Before(oldest[maxIdx].ts) {
					oldest[maxIdx] = entry{id, ts}
				}
			}
		}
		for _, e := range oldest {
			delete(g.sentMessages, e.id)
		}
	}

	// Evict expired receivedDigests
	for id, ts := range g.receivedDigests {
		if ts.Before(cutoff) {
			delete(g.receivedDigests, id)
		}
	}
}

// GetCacheSize returns the current cache size
func (g *GossipEngine) GetCacheSize() int {
	return g.cache.Size()
}

// Close shuts down the gossip engine
func (g *GossipEngine) Close() error {
	g.cancel()
	return nil
}

// Helper functions

// generateMessageID generates a unique ID for a message
func generateMessageID(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

// selectRandomPeers selects n random peers from the list
func selectRandomPeers(peers []peer.ID, n int) []peer.ID {
	if n >= len(peers) {
		return peers
	}

	// Fisher-Yates shuffle
	shuffled := make([]peer.ID, len(peers))
	copy(shuffled, peers)

	for i := len(shuffled) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:n]
}

// serializeMessage serializes a message (simplified)
func serializeMessage(msg *Message) []byte {
	// In production, use protobuf
	// For now, return payload with header
	return msg.Payload
}

// serializeDigests serializes message digests (simplified)
func serializeDigests(digests []string) []byte {
	// In production, use protobuf
	return []byte(strings.Join(digests, ","))
}
