package gossip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMessageCache tests cache creation
func TestNewMessageCache(t *testing.T) {
	t.Run("CreateCache", func(t *testing.T) {
		cache := NewMessageCache(100)
		require.NotNil(t, cache)
		assert.Equal(t, 100, cache.capacity)
		assert.Equal(t, 0, cache.Size())
		assert.NotNil(t, cache.bloom)
	})

	t.Run("CreateCacheWithDifferentCapacities", func(t *testing.T) {
		capacities := []int{10, 100, 1000, 10000}
		for _, cap := range capacities {
			cache := NewMessageCache(cap)
			assert.Equal(t, cap, cache.capacity)
		}
	})
}

// TestCacheAdd tests adding messages to cache
func TestCacheAdd(t *testing.T) {
	t.Run("AddSingleMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		msg := &Message{
			ID:        "msg-001",
			Payload:   []byte("test payload"),
			TTL:       10,
			Timestamp: time.Now().Unix(),
		}

		cache.Add(msg)
		assert.Equal(t, 1, cache.Size())
	})

	t.Run("AddMultipleMessages", func(t *testing.T) {
		cache := NewMessageCache(100)

		for i := 0; i < 10; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		assert.Equal(t, 10, cache.Size())
	})

	t.Run("AddDuplicateMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		msg := &Message{
			ID:        "duplicate",
			Payload:   []byte("test"),
			TTL:       10,
			Timestamp: time.Now().Unix(),
		}

		cache.Add(msg)
		cache.Add(msg) // Add again

		// Size should still be 1
		assert.Equal(t, 1, cache.Size())
	})

	t.Run("AddUpdatesExistingMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		msg1 := &Message{
			ID:        "update-test",
			Payload:   []byte("old payload"),
			TTL:       10,
			Timestamp: 100,
		}

		msg2 := &Message{
			ID:        "update-test",
			Payload:   []byte("new payload"),
			TTL:       5,
			Timestamp: 200,
		}

		cache.Add(msg1)
		cache.Add(msg2)

		retrieved, exists := cache.Get("update-test")
		require.True(t, exists)
		assert.Equal(t, []byte("new payload"), retrieved.Payload)
		assert.Equal(t, int64(200), retrieved.Timestamp)
	})
}

// TestCacheEviction tests LRU eviction
func TestCacheEviction(t *testing.T) {
	t.Run("EvictLRUWhenFull", func(t *testing.T) {
		cache := NewMessageCache(3) // Small capacity

		// Add 4 messages
		for i := 0; i < 4; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		// Cache should only have 3 messages
		assert.Equal(t, 3, cache.Size())

		// First message should be evicted
		contains := cache.Contains("msg-0")
		assert.False(t, contains)

		// Other messages should still be present
		assert.True(t, cache.Contains("msg-1"))
		assert.True(t, cache.Contains("msg-2"))
		assert.True(t, cache.Contains("msg-3"))
	})

	t.Run("AccessUpdatesLRU", func(t *testing.T) {
		cache := NewMessageCache(3)

		// Add 3 messages
		for i := 0; i < 3; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		// Access the first message to make it recently used
		cache.Get("msg-0")

		// Add a new message (should evict msg-1)
		cache.Add(&Message{
			ID:        "msg-3",
			Payload:   []byte("test"),
			TTL:       10,
			Timestamp: time.Now().Unix(),
		})

		// msg-0 should still be present (was accessed)
		assert.True(t, cache.Contains("msg-0"))

		// msg-1 should be evicted (least recently used)
		assert.False(t, cache.Contains("msg-1"))
	})
}

// TestCacheContains tests cache lookup
func TestCacheContains(t *testing.T) {
	t.Run("ContainsExistingMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		msg := &Message{
			ID:        "exists",
			Payload:   []byte("test"),
			TTL:       10,
			Timestamp: time.Now().Unix(),
		}

		cache.Add(msg)
		assert.True(t, cache.Contains("exists"))
	})

	t.Run("ContainsNonExistentMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		assert.False(t, cache.Contains("non-existent"))
	})

	t.Run("ContainsUsesBloomFilter", func(t *testing.T) {
		cache := NewMessageCache(100)

		// Add messages
		for i := 0; i < 10; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		// Check bloom filter is working
		for i := 0; i < 10; i++ {
			msgID := "msg-" + string(rune('0'+i))
			assert.True(t, cache.bloom.Contains(msgID))
		}
	})
}

// TestCacheGet tests message retrieval
func TestCacheGet(t *testing.T) {
	t.Run("GetExistingMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		msg := &Message{
			ID:        "get-test",
			Payload:   []byte("test payload"),
			TTL:       10,
			Timestamp: 12345,
		}

		cache.Add(msg)

		retrieved, exists := cache.Get("get-test")
		require.True(t, exists)
		assert.Equal(t, msg.ID, retrieved.ID)
		assert.Equal(t, msg.Payload, retrieved.Payload)
		assert.Equal(t, msg.Timestamp, retrieved.Timestamp)
	})

	t.Run("GetNonExistentMessage", func(t *testing.T) {
		cache := NewMessageCache(100)

		retrieved, exists := cache.Get("non-existent")
		assert.False(t, exists)
		assert.Nil(t, retrieved)
	})

	t.Run("GetMovesToFront", func(t *testing.T) {
		cache := NewMessageCache(3)

		// Add 3 messages
		for i := 0; i < 3; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		// Get msg-0 to move it to front
		cache.Get("msg-0")

		// Add another message (should evict msg-1, not msg-0)
		cache.Add(&Message{
			ID:        "msg-3",
			Payload:   []byte("test"),
			TTL:       10,
			Timestamp: time.Now().Unix(),
		})

		// msg-0 should still exist
		_, exists := cache.Get("msg-0")
		assert.True(t, exists)

		// msg-1 should be evicted
		_, exists = cache.Get("msg-1")
		assert.False(t, exists)
	})
}

// TestGetDigests tests digest retrieval
func TestGetDigests(t *testing.T) {
	t.Run("GetDigestsFromEmptyCache", func(t *testing.T) {
		cache := NewMessageCache(100)

		digests := cache.GetDigests()
		assert.Empty(t, digests)
	})

	t.Run("GetDigestsFromPopulatedCache", func(t *testing.T) {
		cache := NewMessageCache(100)

		// Add messages
		msgIDs := []string{"msg-1", "msg-2", "msg-3"}
		for _, id := range msgIDs {
			msg := &Message{
				ID:        id,
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		digests := cache.GetDigests()
		assert.Len(t, digests, 3)

		// Verify all IDs are present
		digestMap := make(map[string]bool)
		for _, d := range digests {
			digestMap[d] = true
		}

		for _, id := range msgIDs {
			assert.True(t, digestMap[id])
		}
	})
}

// TestCacheClear tests cache clearing
func TestCacheClear(t *testing.T) {
	t.Run("ClearPopulatedCache", func(t *testing.T) {
		cache := NewMessageCache(100)

		// Add messages
		for i := 0; i < 10; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		assert.Equal(t, 10, cache.Size())

		cache.Clear()

		assert.Equal(t, 0, cache.Size())

		// Verify messages are gone
		for i := 0; i < 10; i++ {
			msgID := "msg-" + string(rune('0'+i))
			assert.False(t, cache.Contains(msgID))
		}
	})

	t.Run("ClearEmptyCache", func(t *testing.T) {
		cache := NewMessageCache(100)

		cache.Clear()
		assert.Equal(t, 0, cache.Size())
	})
}

// TestBloomFilter tests bloom filter functionality
func TestBloomFilter(t *testing.T) {
	t.Run("CreateBloomFilter", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)
		require.NotNil(t, bf)
		assert.Greater(t, bf.size, 0)
		assert.Greater(t, bf.hashCount, 0)
	})

	t.Run("AddAndContains", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)

		bf.Add("test-key")
		assert.True(t, bf.Contains("test-key"))
	})

	t.Run("DoesNotContainNonExistent", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)

		bf.Add("key-1")
		bf.Add("key-2")

		// Should not contain keys not added
		assert.False(t, bf.Contains("key-3"))
	})

	t.Run("AddMultipleKeys", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)

		keys := []string{"key-1", "key-2", "key-3", "key-4", "key-5"}

		for _, key := range keys {
			bf.Add(key)
		}

		for _, key := range keys {
			assert.True(t, bf.Contains(key))
		}
	})

	t.Run("HashConsistency", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)

		hashes1 := bf.getHashes("consistent-key")
		hashes2 := bf.getHashes("consistent-key")

		assert.Equal(t, hashes1, hashes2)
	})

	t.Run("DifferentKeysGenerateDifferentHashes", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)

		hashes1 := bf.getHashes("key-1")
		hashes2 := bf.getHashes("key-2")

		// At least one hash should be different
		different := false
		for i := 0; i < len(hashes1); i++ {
			if hashes1[i] != hashes2[i] {
				different = true
				break
			}
		}
		assert.True(t, different)
	})

	t.Run("CorrectNumberOfHashes", func(t *testing.T) {
		bf := NewBloomFilter(1000, 0.01)

		hashes := bf.getHashes("test")
		assert.Equal(t, bf.hashCount, len(hashes))
	})
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	t.Run("ConcurrentAdd", func(t *testing.T) {
		cache := NewMessageCache(1000)

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					msg := &Message{
						ID:        "concurrent-" + string(rune('0'+id)) + "-" + string(rune('0'+j)),
						Payload:   []byte("test"),
						TTL:       10,
						Timestamp: time.Now().Unix(),
					}
					cache.Add(msg)
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		assert.Equal(t, 100, cache.Size())
	})

	t.Run("ConcurrentContains", func(t *testing.T) {
		cache := NewMessageCache(100)

		// Pre-populate cache
		for i := 0; i < 10; i++ {
			msg := &Message{
				ID:        "msg-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		done := make(chan bool)
		for i := 0; i < 20; i++ {
			go func() {
				for j := 0; j < 10; j++ {
					msgID := "msg-" + string(rune('0'+j))
					cache.Contains(msgID)
				}
				done <- true
			}()
		}

		for i := 0; i < 20; i++ {
			<-done
		}
	})

	t.Run("ConcurrentGetAndAdd", func(t *testing.T) {
		cache := NewMessageCache(100)

		done := make(chan bool)

		// Concurrent adds
		for i := 0; i < 5; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					msg := &Message{
						ID:        "add-" + string(rune('0'+id)) + "-" + string(rune('0'+j)),
						Payload:   []byte("test"),
						TTL:       10,
						Timestamp: time.Now().Unix(),
					}
					cache.Add(msg)
				}
				done <- true
			}(i)
		}

		// Concurrent gets
		for i := 0; i < 5; i++ {
			go func(id int) {
				for j := 0; j < 10; j++ {
					msgID := "add-" + string(rune('0'+id)) + "-" + string(rune('0'+j))
					cache.Get(msgID)
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// TestCacheSize tests size tracking
func TestCacheSize(t *testing.T) {
	t.Run("SizeIncreasesWithAdd", func(t *testing.T) {
		cache := NewMessageCache(100)

		for i := 0; i < 10; i++ {
			msg := &Message{
				ID:        "size-test-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
			assert.Equal(t, i+1, cache.Size())
		}
	})

	t.Run("SizeDoesNotExceedCapacity", func(t *testing.T) {
		capacity := 10
		cache := NewMessageCache(capacity)

		for i := 0; i < capacity*2; i++ {
			msg := &Message{
				ID:        "overflow-" + string(rune('0'+i%10)) + string(rune('A'+i/10)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		assert.LessOrEqual(t, cache.Size(), capacity)
	})

	t.Run("SizeAfterClear", func(t *testing.T) {
		cache := NewMessageCache(100)

		for i := 0; i < 10; i++ {
			msg := &Message{
				ID:        "clear-size-" + string(rune('0'+i)),
				Payload:   []byte("test"),
				TTL:       10,
				Timestamp: time.Now().Unix(),
			}
			cache.Add(msg)
		}

		cache.Clear()
		assert.Equal(t, 0, cache.Size())
	})
}
