package gossip

import (
	"container/list"
	"sync"
)

// MessageCache implements an LRU cache with bloom filter for message deduplication
type MessageCache struct {
	capacity int
	cache    map[string]*list.Element
	lru      *list.List
	mu       sync.RWMutex
	bloom    *BloomFilter
}

// cacheEntry represents a cache entry
type cacheEntry struct {
	key     string
	message *Message
}

// NewMessageCache creates a new message cache
func NewMessageCache(capacity int) *MessageCache {
	return &MessageCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lru:      list.New(),
		bloom:    NewBloomFilter(capacity, BloomFilterFPRate),
	}
}

// Add adds a message to the cache
func (mc *MessageCache) Add(msg *Message) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Check if already exists
	if elem, exists := mc.cache[msg.ID]; exists {
		mc.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).message = msg
		return
	}

	// Add to bloom filter
	mc.bloom.Add(msg.ID)

	// Create new entry
	entry := &cacheEntry{
		key:     msg.ID,
		message: msg,
	}

	// Add to front of LRU
	elem := mc.lru.PushFront(entry)
	mc.cache[msg.ID] = elem

	// Evict if over capacity
	if mc.lru.Len() > mc.capacity {
		mc.evict()
	}
}

// Contains checks if a message ID is in the cache
func (mc *MessageCache) Contains(msgID string) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Quick check with bloom filter
	if !mc.bloom.Contains(msgID) {
		return false
	}

	// Verify with actual cache
	_, exists := mc.cache[msgID]
	return exists
}

// Get retrieves a message from the cache
func (mc *MessageCache) Get(msgID string) (*Message, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if elem, exists := mc.cache[msgID]; exists {
		mc.lru.MoveToFront(elem)
		return elem.Value.(*cacheEntry).message, true
	}

	return nil, false
}

// GetDigests returns a list of all message IDs in the cache
func (mc *MessageCache) GetDigests() []string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	digests := make([]string, 0, len(mc.cache))
	for key := range mc.cache {
		digests = append(digests, key)
	}
	return digests
}

// Size returns the number of messages in the cache
func (mc *MessageCache) Size() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.lru.Len()
}

// evict removes the least recently used item
func (mc *MessageCache) evict() {
	elem := mc.lru.Back()
	if elem != nil {
		mc.lru.Remove(elem)
		entry := elem.Value.(*cacheEntry)
		delete(mc.cache, entry.key)
	}
}

// Clear removes all items from the cache
func (mc *MessageCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.cache = make(map[string]*list.Element)
	mc.lru = list.New()
	mc.bloom = NewBloomFilter(mc.capacity, BloomFilterFPRate)
}

// BloomFilter implements a simple bloom filter
type BloomFilter struct {
	bits      []bool
	size      int
	hashCount int
}

// NewBloomFilter creates a new bloom filter
func NewBloomFilter(capacity int, fpRate float64) *BloomFilter {
	// Calculate optimal size and hash count
	// m = -n*ln(p) / (ln(2)^2)
	// k = (m/n) * ln(2)

	size := capacity * 10 // Simplified: 10 bits per element
	hashCount := 7        // Simplified: use 7 hash functions

	return &BloomFilter{
		bits:      make([]bool, size),
		size:      size,
		hashCount: hashCount,
	}
}

// Add adds an element to the bloom filter
func (bf *BloomFilter) Add(key string) {
	hashes := bf.getHashes(key)
	for _, h := range hashes {
		bf.bits[h%bf.size] = true
	}
}

// Contains checks if an element might be in the set
func (bf *BloomFilter) Contains(key string) bool {
	hashes := bf.getHashes(key)
	for _, h := range hashes {
		if !bf.bits[h%bf.size] {
			return false
		}
	}
	return true
}

// getHashes generates multiple hash values for a key
func (bf *BloomFilter) getHashes(key string) []int {
	hashes := make([]int, bf.hashCount)

	// Simple hash function (in production, use better hashing)
	hash := 0
	for i, c := range key {
		hash = hash*31 + int(c) + i
	}

	// Generate k different hashes
	for i := 0; i < bf.hashCount; i++ {
		hashes[i] = (hash + i*i) & 0x7fffffff // Keep positive
	}

	return hashes
}
