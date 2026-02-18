package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
)

var (
	// BucketTickets stores ticket states
	BucketTickets = []byte("tickets")
	// BucketConsensus stores consensus logs
	BucketConsensus = []byte("consensus")
	// BucketCheckpoints stores checkpoints
	BucketCheckpoints = []byte("checkpoints")
	// BucketMetadata stores metadata
	BucketMetadata = []byte("metadata")
)

// Store handles persistent storage using BoltDB
type Store struct {
	db   *bolt.DB
	path string
}

// TicketRecord represents a ticket stored in the database
type TicketRecord struct {
	ID          string            `json:"id"`
	State       string            `json:"state"`
	Data        []byte            `json:"data"`
	ValidatorID string            `json:"validator_id"`
	Timestamp   int64             `json:"timestamp"`
	VectorClock map[string]uint64 `json:"vector_clock"`
	Metadata    map[string]string `json:"metadata"`
}

// ConsensusRecord represents a consensus log entry
type ConsensusRecord struct {
	Sequence  int64  `json:"sequence"`
	View      int64  `json:"view"`
	RequestID string `json:"request_id"`
	TicketID  string `json:"ticket_id"`
	Operation string `json:"operation"`
	Digest    string `json:"digest"`
	Timestamp int64  `json:"timestamp"`
}

// CheckpointRecord represents a checkpoint
type CheckpointRecord struct {
	Sequence    int64  `json:"sequence"`
	StateDigest string `json:"state_digest"`
	NodeID      string `json:"node_id"`
	Timestamp   int64  `json:"timestamp"`
}

// Config holds storage configuration
type Config struct {
	Path    string
	Timeout time.Duration
}

// NewStore creates a new BoltDB store
func NewStore(cfg *Config) (*Store, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	db, err := bolt.Open(cfg.Path, 0600, &bolt.Options{Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{BucketTickets, BucketConsensus, BucketCheckpoints, BucketMetadata}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Store{
		db:   db,
		path: cfg.Path,
	}, nil
}

// SaveTicket saves a ticket to the database
func (s *Store) SaveTicket(ticket *TicketRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTickets)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketTickets)
		}

		data, err := json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("failed to marshal ticket: %w", err)
		}

		return b.Put([]byte(ticket.ID), data)
	})
}

// GetTicket retrieves a ticket from the database
func (s *Store) GetTicket(ticketID string) (*TicketRecord, error) {
	var ticket *TicketRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTickets)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketTickets)
		}

		data := b.Get([]byte(ticketID))
		if data == nil {
			return fmt.Errorf("ticket %s not found", ticketID)
		}

		ticket = &TicketRecord{}
		return json.Unmarshal(data, ticket)
	})

	return ticket, err
}

// GetAllTickets retrieves all tickets from the database
func (s *Store) GetAllTickets() ([]*TicketRecord, error) {
	var tickets []*TicketRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTickets)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketTickets)
		}

		// Pre-allocate with exact count from bucket stats
		tickets = make([]*TicketRecord, 0, b.Stats().KeyN)

		return b.ForEach(func(k, v []byte) error {
			ticket := &TicketRecord{}
			if err := json.Unmarshal(v, ticket); err != nil {
				return err
			}
			tickets = append(tickets, ticket)
			return nil
		})
	})

	return tickets, err
}

// DeleteTicket deletes a ticket from the database
func (s *Store) DeleteTicket(ticketID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTickets)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketTickets)
		}
		return b.Delete([]byte(ticketID))
	})
}

// SaveConsensusLog saves a consensus log entry
func (s *Store) SaveConsensusLog(record *ConsensusRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketConsensus)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketConsensus)
		}

		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal consensus record: %w", err)
		}

		// Use sequence number as key
		return b.Put(sequenceKey(record.Sequence), data)
	})
}

// GetConsensusLog retrieves a consensus log entry by sequence number
func (s *Store) GetConsensusLog(sequence int64) (*ConsensusRecord, error) {
	var record *ConsensusRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketConsensus)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketConsensus)
		}

		data := b.Get(sequenceKey(sequence))
		if data == nil {
			return fmt.Errorf("consensus log %d not found", sequence)
		}

		record = &ConsensusRecord{}
		return json.Unmarshal(data, record)
	})

	return record, err
}

// GetConsensusLogs retrieves all consensus logs
func (s *Store) GetConsensusLogs() ([]*ConsensusRecord, error) {
	var logs []*ConsensusRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketConsensus)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketConsensus)
		}

		return b.ForEach(func(k, v []byte) error {
			record := &ConsensusRecord{}
			if err := json.Unmarshal(v, record); err != nil {
				return err
			}
			logs = append(logs, record)
			return nil
		})
	})

	return logs, err
}

// SaveCheckpoint saves a checkpoint
func (s *Store) SaveCheckpoint(checkpoint *CheckpointRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCheckpoints)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketCheckpoints)
		}

		data, err := json.Marshal(checkpoint)
		if err != nil {
			return fmt.Errorf("failed to marshal checkpoint: %w", err)
		}

		return b.Put(sequenceKey(checkpoint.Sequence), data)
	})
}

// GetLatestCheckpoint retrieves the latest checkpoint
func (s *Store) GetLatestCheckpoint() (*CheckpointRecord, error) {
	var checkpoint *CheckpointRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCheckpoints)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketCheckpoints)
		}

		c := b.Cursor()
		k, v := c.Last()
		if k == nil {
			return fmt.Errorf("no checkpoints found")
		}

		checkpoint = &CheckpointRecord{}
		return json.Unmarshal(v, checkpoint)
	})

	return checkpoint, err
}

// SetMetadata stores a metadata key-value pair
func (s *Store) SetMetadata(key string, value []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMetadata)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketMetadata)
		}
		return b.Put([]byte(key), value)
	})
}

// GetMetadata retrieves a metadata value
func (s *Store) GetMetadata(key string) ([]byte, error) {
	var value []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMetadata)
		if b == nil {
			return fmt.Errorf("bucket %s not found", BucketMetadata)
		}

		data := b.Get([]byte(key))
		if data == nil {
			return fmt.Errorf("metadata key %s not found", key)
		}

		value = make([]byte, len(data))
		copy(value, data)
		return nil
	})

	return value, err
}

// Backup creates a backup of the database
func (s *Store) Backup(path string) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(path, 0600)
	})
}

// GetStats returns database statistics
func (s *Store) GetStats() map[string]interface{} {
	stats := s.db.Stats()
	return map[string]interface{}{
		"tx_count":       stats.TxN,
		"open_tx_count":  stats.OpenTxN,
		"page_count":     stats.TxStats.PageCount,
		"page_alloc":     stats.TxStats.PageAlloc,
		"cursor_count":   stats.TxStats.CursorCount,
		"node_count":     stats.TxStats.NodeCount,
		"node_deref":     stats.TxStats.NodeDeref,
		"rebalance":      stats.TxStats.Rebalance,
		"rebalance_time": stats.TxStats.RebalanceTime,
		"split":          stats.TxStats.Split,
		"spill":          stats.TxStats.Spill,
		"spill_time":     stats.TxStats.SpillTime,
		"write":          stats.TxStats.Write,
		"write_time":     stats.TxStats.WriteTime,
	}
}

// sequenceKey converts an int64 sequence number to a zero-padded key.
// Uses strconv instead of fmt.Sprintf for lower allocation overhead.
// Max int64 is 19 digits, so we use a 20-byte buffer for safety.
func sequenceKey(seq int64) []byte {
	const width = 20
	s := strconv.FormatInt(seq, 10)
	if len(s) >= width {
		return []byte(s)
	}
	var key [width]byte
	for i := range key {
		key[i] = '0'
	}
	copy(key[width-len(s):], s)
	return key[:]
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// Compact performs database compaction (not directly supported by BoltDB)
func (s *Store) Compact(newPath string) error {
	// Create new database
	newDB, err := bolt.Open(newPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("failed to create new database: %w", err)
	}
	defer newDB.Close()

	// Copy all buckets
	return s.db.View(func(tx *bolt.Tx) error {
		return newDB.Update(func(newTx *bolt.Tx) error {
			return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
				newBucket, err := newTx.CreateBucketIfNotExists(name)
				if err != nil {
					return err
				}

				return b.ForEach(func(k, v []byte) error {
					return newBucket.Put(k, v)
				})
			})
		})
	})
}
