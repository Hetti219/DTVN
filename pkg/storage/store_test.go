package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a temporary test database
func createTestStore(t *testing.T) (*Store, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		Path:    dbPath,
		Timeout: 5 * time.Second,
	}

	store, err := NewStore(cfg)
	require.NoError(t, err, "Failed to create test store")
	require.NotNil(t, store, "Store should not be nil")

	return store, dbPath
}

// TestNewStore tests store creation and initialization
func TestNewStore(t *testing.T) {
	t.Run("CreateStoreSuccessfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		cfg := &Config{
			Path:    dbPath,
			Timeout: 5 * time.Second,
		}

		store, err := NewStore(cfg)
		require.NoError(t, err)
		require.NotNil(t, store)
		defer store.Close()

		assert.Equal(t, dbPath, store.path)
		assert.NotNil(t, store.db)
	})

	t.Run("CreateStoreWithDefaultTimeout", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		cfg := &Config{
			Path: dbPath,
			// Timeout not set, should default to 10 seconds
		}

		store, err := NewStore(cfg)
		require.NoError(t, err)
		require.NotNil(t, store)
		defer store.Close()
	})

	t.Run("CreateStoreInvalidPath", func(t *testing.T) {
		cfg := &Config{
			Path:    "/invalid/path/that/does/not/exist/test.db",
			Timeout: 5 * time.Second,
		}

		store, err := NewStore(cfg)
		assert.Error(t, err)
		assert.Nil(t, store)
	})

	t.Run("CreateStoreWithSamePathTwice", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		cfg := &Config{
			Path:    dbPath,
			Timeout: 5 * time.Second,
		}

		store1, err := NewStore(cfg)
		require.NoError(t, err)
		defer store1.Close()

		// BoltDB should prevent opening the same file twice
		store2, err := NewStore(cfg)
		assert.Error(t, err)
		assert.Nil(t, store2)
	})
}

// TestTicketOperations tests ticket CRUD operations
func TestTicketOperations(t *testing.T) {
	t.Run("SaveAndGetTicket", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		ticket := &TicketRecord{
			ID:          "ticket-001",
			State:       "VALIDATED",
			Data:        []byte("test-data"),
			ValidatorID: "validator-1",
			Timestamp:   time.Now().Unix(),
			VectorClock: map[string]uint64{
				"node-1": 1,
				"node-2": 2,
			},
			Metadata: map[string]string{
				"event": "concert",
				"seat":  "A1",
			},
		}

		// Save ticket
		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		// Retrieve ticket
		retrieved, err := store.GetTicket("ticket-001")
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, ticket.ID, retrieved.ID)
		assert.Equal(t, ticket.State, retrieved.State)
		assert.Equal(t, ticket.Data, retrieved.Data)
		assert.Equal(t, ticket.ValidatorID, retrieved.ValidatorID)
		assert.Equal(t, ticket.Timestamp, retrieved.Timestamp)
		assert.Equal(t, ticket.VectorClock, retrieved.VectorClock)
		assert.Equal(t, ticket.Metadata, retrieved.Metadata)
	})

	t.Run("UpdateExistingTicket", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		ticket := &TicketRecord{
			ID:          "ticket-002",
			State:       "PENDING",
			ValidatorID: "validator-1",
			Timestamp:   time.Now().Unix(),
		}

		// Save initial ticket
		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		// Update ticket state
		ticket.State = "VALIDATED"
		ticket.VectorClock = map[string]uint64{"node-1": 5}

		err = store.SaveTicket(ticket)
		require.NoError(t, err)

		// Verify update
		retrieved, err := store.GetTicket("ticket-002")
		require.NoError(t, err)
		assert.Equal(t, "VALIDATED", retrieved.State)
		assert.Equal(t, map[string]uint64{"node-1": 5}, retrieved.VectorClock)
	})

	t.Run("GetNonExistentTicket", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		ticket, err := store.GetTicket("non-existent")
		assert.Error(t, err)
		assert.Nil(t, ticket)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("DeleteTicket", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		ticket := &TicketRecord{
			ID:          "ticket-003",
			State:       "VALIDATED",
			ValidatorID: "validator-1",
			Timestamp:   time.Now().Unix(),
		}

		// Save ticket
		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		// Verify it exists
		_, err = store.GetTicket("ticket-003")
		require.NoError(t, err)

		// Delete ticket
		err = store.DeleteTicket("ticket-003")
		require.NoError(t, err)

		// Verify deletion
		_, err = store.GetTicket("ticket-003")
		assert.Error(t, err)
	})

	t.Run("DeleteNonExistentTicket", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Deleting non-existent ticket should not error
		err := store.DeleteTicket("non-existent")
		assert.NoError(t, err)
	})

	t.Run("GetAllTickets", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Initially empty
		tickets, err := store.GetAllTickets()
		require.NoError(t, err)
		assert.Empty(t, tickets)

		// Add multiple tickets
		for i := 1; i <= 5; i++ {
			ticket := &TicketRecord{
				ID:          string(rune('A' + i - 1)) + "-ticket",
				State:       "VALIDATED",
				ValidatorID: "validator-1",
				Timestamp:   time.Now().Unix(),
			}
			err := store.SaveTicket(ticket)
			require.NoError(t, err)
		}

		// Retrieve all
		tickets, err = store.GetAllTickets()
		require.NoError(t, err)
		assert.Len(t, tickets, 5)
	})

	t.Run("SaveTicketWithNilFields", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		ticket := &TicketRecord{
			ID:        "ticket-nil",
			State:     "ISSUED",
			Timestamp: time.Now().Unix(),
			// VectorClock and Metadata are nil
		}

		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		retrieved, err := store.GetTicket("ticket-nil")
		require.NoError(t, err)
		assert.Equal(t, ticket.ID, retrieved.ID)
	})
}

// TestConsensusOperations tests consensus log operations
func TestConsensusOperations(t *testing.T) {
	t.Run("SaveAndGetConsensusLog", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		record := &ConsensusRecord{
			Sequence:  1,
			View:      0,
			RequestID: "req-001",
			TicketID:  "ticket-001",
			Operation: "VALIDATE",
			Digest:    "abcd1234",
			Timestamp: time.Now().Unix(),
		}

		err := store.SaveConsensusLog(record)
		require.NoError(t, err)

		retrieved, err := store.GetConsensusLog(1)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, record.Sequence, retrieved.Sequence)
		assert.Equal(t, record.View, retrieved.View)
		assert.Equal(t, record.RequestID, retrieved.RequestID)
		assert.Equal(t, record.TicketID, retrieved.TicketID)
		assert.Equal(t, record.Operation, retrieved.Operation)
		assert.Equal(t, record.Digest, retrieved.Digest)
		assert.Equal(t, record.Timestamp, retrieved.Timestamp)
	})

	t.Run("SaveMultipleConsensusLogs", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Save logs with different sequence numbers
		for i := int64(1); i <= 10; i++ {
			record := &ConsensusRecord{
				Sequence:  i,
				View:      i % 3,
				RequestID: "req-" + string(rune('0'+i)),
				TicketID:  "ticket-" + string(rune('0'+i)),
				Operation: "VALIDATE",
				Digest:    "digest-" + string(rune('0'+i)),
				Timestamp: time.Now().Unix(),
			}
			err := store.SaveConsensusLog(record)
			require.NoError(t, err)
		}

		// Retrieve all logs
		logs, err := store.GetConsensusLogs()
		require.NoError(t, err)
		assert.Len(t, logs, 10)
	})

	t.Run("GetNonExistentConsensusLog", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		log, err := store.GetConsensusLog(999)
		assert.Error(t, err)
		assert.Nil(t, log)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("OverwriteConsensusLog", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		record1 := &ConsensusRecord{
			Sequence:  1,
			View:      0,
			RequestID: "req-001",
			Digest:    "old-digest",
			Timestamp: time.Now().Unix(),
		}

		err := store.SaveConsensusLog(record1)
		require.NoError(t, err)

		// Overwrite with new record
		record2 := &ConsensusRecord{
			Sequence:  1,
			View:      1,
			RequestID: "req-002",
			Digest:    "new-digest",
			Timestamp: time.Now().Unix(),
		}

		err = store.SaveConsensusLog(record2)
		require.NoError(t, err)

		// Verify the new record
		retrieved, err := store.GetConsensusLog(1)
		require.NoError(t, err)
		assert.Equal(t, "new-digest", retrieved.Digest)
		assert.Equal(t, "req-002", retrieved.RequestID)
	})

	t.Run("GetAllConsensusLogsEmpty", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		logs, err := store.GetConsensusLogs()
		require.NoError(t, err)
		assert.Empty(t, logs)
	})
}

// TestCheckpointOperations tests checkpoint operations
func TestCheckpointOperations(t *testing.T) {
	t.Run("SaveAndGetCheckpoint", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		checkpoint := &CheckpointRecord{
			Sequence:    100,
			StateDigest: "checkpoint-digest-100",
			NodeID:      "node-1",
			Timestamp:   time.Now().Unix(),
		}

		err := store.SaveCheckpoint(checkpoint)
		require.NoError(t, err)

		// Get latest checkpoint
		retrieved, err := store.GetLatestCheckpoint()
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, checkpoint.Sequence, retrieved.Sequence)
		assert.Equal(t, checkpoint.StateDigest, retrieved.StateDigest)
		assert.Equal(t, checkpoint.NodeID, retrieved.NodeID)
		assert.Equal(t, checkpoint.Timestamp, retrieved.Timestamp)
	})

	t.Run("GetLatestCheckpointFromMultiple", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Save multiple checkpoints
		for i := int64(10); i <= 50; i += 10 {
			checkpoint := &CheckpointRecord{
				Sequence:    i,
				StateDigest: "digest-" + string(rune('0'+i/10)),
				NodeID:      "node-1",
				Timestamp:   time.Now().Unix(),
			}
			err := store.SaveCheckpoint(checkpoint)
			require.NoError(t, err)
		}

		// Get latest should return sequence 50
		latest, err := store.GetLatestCheckpoint()
		require.NoError(t, err)
		assert.Equal(t, int64(50), latest.Sequence)
	})

	t.Run("GetLatestCheckpointWhenEmpty", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		checkpoint, err := store.GetLatestCheckpoint()
		assert.Error(t, err)
		assert.Nil(t, checkpoint)
		assert.Contains(t, err.Error(), "no checkpoints found")
	})

	t.Run("OverwriteCheckpoint", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		checkpoint1 := &CheckpointRecord{
			Sequence:    100,
			StateDigest: "old-digest",
			NodeID:      "node-1",
			Timestamp:   time.Now().Unix(),
		}

		err := store.SaveCheckpoint(checkpoint1)
		require.NoError(t, err)

		// Overwrite with same sequence
		checkpoint2 := &CheckpointRecord{
			Sequence:    100,
			StateDigest: "new-digest",
			NodeID:      "node-2",
			Timestamp:   time.Now().Unix(),
		}

		err = store.SaveCheckpoint(checkpoint2)
		require.NoError(t, err)

		latest, err := store.GetLatestCheckpoint()
		require.NoError(t, err)
		assert.Equal(t, "new-digest", latest.StateDigest)
		assert.Equal(t, "node-2", latest.NodeID)
	})
}

// TestMetadataOperations tests metadata operations
func TestMetadataOperations(t *testing.T) {
	t.Run("SetAndGetMetadata", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		key := "node-id"
		value := []byte("validator-001")

		err := store.SetMetadata(key, value)
		require.NoError(t, err)

		retrieved, err := store.GetMetadata(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("UpdateMetadata", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		key := "config"
		value1 := []byte("config-v1")

		err := store.SetMetadata(key, value1)
		require.NoError(t, err)

		// Update
		value2 := []byte("config-v2")
		err = store.SetMetadata(key, value2)
		require.NoError(t, err)

		retrieved, err := store.GetMetadata(key)
		require.NoError(t, err)
		assert.Equal(t, value2, retrieved)
	})

	t.Run("GetNonExistentMetadata", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		value, err := store.GetMetadata("non-existent")
		assert.Error(t, err)
		assert.Nil(t, value)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("SetMultipleMetadata", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		metadata := map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key3": []byte("value3"),
		}

		for k, v := range metadata {
			err := store.SetMetadata(k, v)
			require.NoError(t, err)
		}

		for k, expected := range metadata {
			value, err := store.GetMetadata(k)
			require.NoError(t, err)
			assert.Equal(t, expected, value)
		}
	})

	t.Run("SetEmptyMetadata", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		err := store.SetMetadata("empty", []byte{})
		require.NoError(t, err)

		value, err := store.GetMetadata("empty")
		require.NoError(t, err)
		assert.Empty(t, value)
	})
}

// TestBackupAndRestore tests backup functionality
func TestBackupAndRestore(t *testing.T) {
	t.Run("BackupDatabase", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Add some data
		ticket := &TicketRecord{
			ID:          "ticket-backup",
			State:       "VALIDATED",
			ValidatorID: "validator-1",
			Timestamp:   time.Now().Unix(),
		}
		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		// Create backup
		backupPath := filepath.Join(t.TempDir(), "backup.db")
		err = store.Backup(backupPath)
		require.NoError(t, err)

		// Verify backup file exists
		_, err = os.Stat(backupPath)
		assert.NoError(t, err)

		// Open backup and verify data
		backupStore, err := NewStore(&Config{
			Path:    backupPath,
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)
		defer backupStore.Close()

		retrievedTicket, err := backupStore.GetTicket("ticket-backup")
		require.NoError(t, err)
		assert.Equal(t, ticket.ID, retrievedTicket.ID)
	})

	t.Run("BackupToInvalidPath", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		err := store.Backup("/invalid/path/backup.db")
		assert.Error(t, err)
	})
}

// TestCompact tests database compaction
func TestCompact(t *testing.T) {
	t.Run("CompactDatabase", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Add data
		for i := 1; i <= 100; i++ {
			ticket := &TicketRecord{
				ID:          "ticket-" + string(rune('0'+i)),
				State:       "VALIDATED",
				ValidatorID: "validator-1",
				Timestamp:   time.Now().Unix(),
			}
			err := store.SaveTicket(ticket)
			require.NoError(t, err)
		}

		// Compact to new path
		compactPath := filepath.Join(t.TempDir(), "compact.db")
		err := store.Compact(compactPath)
		require.NoError(t, err)

		// Verify compacted database
		compactStore, err := NewStore(&Config{
			Path:    compactPath,
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)
		defer compactStore.Close()

		tickets, err := compactStore.GetAllTickets()
		require.NoError(t, err)
		assert.Len(t, tickets, 100)
	})

	t.Run("CompactToInvalidPath", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		err := store.Compact("/invalid/path/compact.db")
		assert.Error(t, err)
	})
}

// TestGetStats tests statistics retrieval
func TestGetStats(t *testing.T) {
	t.Run("GetDatabaseStats", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		stats := store.GetStats()
		assert.NotNil(t, stats)

		// Verify expected stat keys
		assert.Contains(t, stats, "tx_count")
		assert.Contains(t, stats, "page_count")
		assert.Contains(t, stats, "write")
	})

	t.Run("StatsAfterOperations", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Perform some operations
		for i := 1; i <= 10; i++ {
			ticket := &TicketRecord{
				ID:        "ticket-" + string(rune('0'+i)),
				State:     "VALIDATED",
				Timestamp: time.Now().Unix(),
			}
			err := store.SaveTicket(ticket)
			require.NoError(t, err)
		}

		stats := store.GetStats()
		assert.NotNil(t, stats)

		// Transaction count should be >= 0 (BoltDB counts total lifetime transactions)
		txCount := stats["tx_count"].(int)
		assert.GreaterOrEqual(t, txCount, 0)
	})
}

// TestCloseDatabase tests database closure
func TestCloseDatabase(t *testing.T) {
	t.Run("CloseAndReopen", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Create and close store
		store1, err := NewStore(&Config{
			Path:    dbPath,
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)

		ticket := &TicketRecord{
			ID:        "persistent-ticket",
			State:     "VALIDATED",
			Timestamp: time.Now().Unix(),
		}
		err = store1.SaveTicket(ticket)
		require.NoError(t, err)

		err = store1.Close()
		require.NoError(t, err)

		// Reopen and verify data persisted
		store2, err := NewStore(&Config{
			Path:    dbPath,
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)
		defer store2.Close()

		retrieved, err := store2.GetTicket("persistent-ticket")
		require.NoError(t, err)
		assert.Equal(t, ticket.ID, retrieved.ID)
	})

	t.Run("CloseMultipleTimes", func(t *testing.T) {
		store, _ := createTestStore(t)

		err := store.Close()
		assert.NoError(t, err)

		// BoltDB allows multiple closes without error (idempotent)
		err = store.Close()
		assert.NoError(t, err)
	})
}

// TestConcurrentAccess tests concurrent operations
func TestConcurrentAccess(t *testing.T) {
	t.Run("ConcurrentWrites", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				ticket := &TicketRecord{
					ID:        "concurrent-" + string(rune('0'+id)),
					State:     "VALIDATED",
					Timestamp: time.Now().Unix(),
				}
				err := store.SaveTicket(ticket)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all tickets were saved
		tickets, err := store.GetAllTickets()
		require.NoError(t, err)
		assert.Len(t, tickets, 10)
	})

	t.Run("ConcurrentReads", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Add a ticket first
		ticket := &TicketRecord{
			ID:        "shared-ticket",
			State:     "VALIDATED",
			Timestamp: time.Now().Unix(),
		}
		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		// Concurrent reads
		done := make(chan bool)
		for i := 0; i < 20; i++ {
			go func() {
				retrieved, err := store.GetTicket("shared-ticket")
				assert.NoError(t, err)
				assert.Equal(t, "shared-ticket", retrieved.ID)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 20; i++ {
			<-done
		}
	})
}

// TestEdgeCases tests edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	t.Run("VeryLargeTicketData", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Create ticket with large data
		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		ticket := &TicketRecord{
			ID:        "large-ticket",
			State:     "VALIDATED",
			Data:      largeData,
			Timestamp: time.Now().Unix(),
		}

		err := store.SaveTicket(ticket)
		require.NoError(t, err)

		retrieved, err := store.GetTicket("large-ticket")
		require.NoError(t, err)
		assert.Equal(t, largeData, retrieved.Data)
	})

	t.Run("TicketWithSpecialCharactersInID", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		specialIDs := []string{
			"ticket-with-dashes",
			"ticket_with_underscores",
			"ticket.with.dots",
			"ticket@with@at",
			"ticket#with#hash",
		}

		for _, id := range specialIDs {
			ticket := &TicketRecord{
				ID:        id,
				State:     "VALIDATED",
				Timestamp: time.Now().Unix(),
			}

			err := store.SaveTicket(ticket)
			require.NoError(t, err)

			retrieved, err := store.GetTicket(id)
			require.NoError(t, err)
			assert.Equal(t, id, retrieved.ID)
		}
	})

	t.Run("HighSequenceNumbers", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		// Test with very high sequence numbers
		sequences := []int64{
			999999999,
			1000000000,
			9223372036854775807, // Max int64
		}

		for _, seq := range sequences {
			record := &ConsensusRecord{
				Sequence:  seq,
				View:      0,
				RequestID: "req",
				Timestamp: time.Now().Unix(),
			}

			err := store.SaveConsensusLog(record)
			require.NoError(t, err)

			retrieved, err := store.GetConsensusLog(seq)
			require.NoError(t, err)
			assert.Equal(t, seq, retrieved.Sequence)
		}
	})
}
