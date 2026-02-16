package types

import (
	"fmt"
	"time"
)

// NodeRole represents the role of a node in the network
type NodeRole string

const (
	RolePrimary   NodeRole = "PRIMARY"
	RoleReplica   NodeRole = "REPLICA"
	RoleBootstrap NodeRole = "BOOTSTRAP"
	RoleObserver  NodeRole = "OBSERVER"
)

// NodeInfo represents information about a node
type NodeInfo struct {
	ID            string
	Role          NodeRole
	Address       string
	Port          int
	PublicKey     []byte
	JoinedAt      time.Time
	LastSeen      time.Time
	IsHealthy     bool
	MessagesSent  int64
	MessagesRecvd int64
}

// NetworkState represents the overall network state
type NetworkState struct {
	TotalNodes      int
	ActiveNodes     int
	HealthyNodes    int
	ConsensusRounds int64
	SuccessRate     float64
	AvgLatencyMs    float64
}

// TicketMetadata represents additional ticket metadata
type TicketMetadata struct {
	EventID      string
	EventName    string
	SeatNumber   string
	Price        float64
	PurchaseDate time.Time
	Owner        string
}

// ValidationResult represents the result of a ticket validation
type ValidationResult struct {
	TicketID    string
	IsValid     bool
	State       string
	ValidatedBy []string
	Timestamp   time.Time
	Message     string
}

// ConsensusMetrics represents consensus performance metrics
type ConsensusMetrics struct {
	TotalRounds      int64
	SuccessfulRounds int64
	FailedRounds     int64
	AvgLatencyMs     float64
	CurrentView      int64
	CurrentSequence  int64
}

// GossipMetrics represents gossip protocol metrics
type GossipMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	CacheHits        int64
	CacheMisses      int64
	CacheSize        int
	Bandwidth        int64
}

// Error types

// ErrTicketNotFound is returned when a ticket is not found
type ErrTicketNotFound struct {
	TicketID string
}

func (e *ErrTicketNotFound) Error() string {
	return "ticket not found: " + e.TicketID
}

// ErrInvalidState is returned when a state transition is invalid
type ErrInvalidState struct {
	TicketID  string
	FromState string
	ToState   string
}

func (e *ErrInvalidState) Error() string {
	return "invalid state transition for ticket " + e.TicketID + ": " + e.FromState + " -> " + e.ToState
}

// ErrConsensusFailure is returned when consensus fails
type ErrConsensusFailure struct {
	Sequence int64
	Reason   string
}

func (e *ErrConsensusFailure) Error() string {
	return fmt.Sprintf("consensus failure at sequence %d: %s", e.Sequence, e.Reason)
}

// ErrInsufficientQuorum is returned when there's insufficient quorum
type ErrInsufficientQuorum struct {
	Required int
	Actual   int
}

func (e *ErrInsufficientQuorum) Error() string {
	return fmt.Sprintf("insufficient quorum: required %d, got %d", e.Required, e.Actual)
}
