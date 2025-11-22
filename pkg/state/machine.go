package state

import (
	"fmt"
	"sync"
	"time"
)

// TicketState represents the state of a ticket
type TicketState string

const (
	StateIssued    TicketState = "ISSUED"
	StatePending   TicketState = "PENDING"
	StateValidated TicketState = "VALIDATED"
	StateConsumed  TicketState = "CONSUMED"
	StateDisputed  TicketState = "DISPUTED"
)

// Ticket represents a ticket with its state
type Ticket struct {
	ID          string
	State       TicketState
	Data        []byte
	ValidatorID string
	Timestamp   int64
	VectorClock *VectorClock
	Metadata    map[string]string
}

// StateTransition represents a state transition
type StateTransition struct {
	TicketID    string
	FromState   TicketState
	ToState     TicketState
	ValidatorID string
	Timestamp   int64
	VectorClock *VectorClock
}

// StateMachine manages ticket states and transitions
type StateMachine struct {
	nodeID       string
	currentState map[string]*Ticket
	transitions  []StateTransition
	vectorClock  *VectorClock
	mu           sync.RWMutex
	handlers     []StateChangeHandler
}

// StateChangeHandler is called when a state changes
type StateChangeHandler func(*Ticket, TicketState, TicketState) error

// NewStateMachine creates a new state machine
func NewStateMachine(nodeID string) *StateMachine {
	return &StateMachine{
		nodeID:       nodeID,
		currentState: make(map[string]*Ticket),
		transitions:  make([]StateTransition, 0),
		vectorClock:  NewVectorClock(nodeID),
	}
}

// ValidateTicket validates a ticket and transitions it to VALIDATED state
func (sm *StateMachine) ValidateTicket(ticketID string, validatorID string, data []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if ticket exists
	ticket, exists := sm.currentState[ticketID]
	if !exists {
		// Create new ticket in PENDING state
		ticket = &Ticket{
			ID:          ticketID,
			State:       StatePending,
			Data:        data,
			ValidatorID: validatorID,
			Timestamp:   time.Now().Unix(),
			VectorClock: sm.vectorClock.Copy(),
			Metadata:    make(map[string]string),
		}
		sm.currentState[ticketID] = ticket
	}

	// Check if ticket is already consumed or disputed
	if ticket.State == StateConsumed {
		return fmt.Errorf("ticket %s already consumed", ticketID)
	}
	if ticket.State == StateDisputed {
		return fmt.Errorf("ticket %s is disputed", ticketID)
	}

	// Transition to VALIDATED
	oldState := ticket.State
	ticket.State = StateValidated
	ticket.ValidatorID = validatorID
	ticket.Timestamp = time.Now().Unix()

	// Increment vector clock
	sm.vectorClock.Increment()
	ticket.VectorClock = sm.vectorClock.Copy()

	// Record transition
	transition := StateTransition{
		TicketID:    ticketID,
		FromState:   oldState,
		ToState:     StateValidated,
		ValidatorID: validatorID,
		Timestamp:   ticket.Timestamp,
		VectorClock: ticket.VectorClock.Copy(),
	}
	sm.transitions = append(sm.transitions, transition)

	// Call handlers
	for _, handler := range sm.handlers {
		if err := handler(ticket, oldState, StateValidated); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	return nil
}

// ConsumeTicket marks a ticket as consumed
func (sm *StateMachine) ConsumeTicket(ticketID string, validatorID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ticket, exists := sm.currentState[ticketID]
	if !exists {
		return fmt.Errorf("ticket %s not found", ticketID)
	}

	if ticket.State != StateValidated {
		return fmt.Errorf("ticket %s is not validated (current state: %s)", ticketID, ticket.State)
	}

	// Transition to CONSUMED
	oldState := ticket.State
	ticket.State = StateConsumed
	ticket.Timestamp = time.Now().Unix()

	// Increment vector clock
	sm.vectorClock.Increment()
	ticket.VectorClock = sm.vectorClock.Copy()

	// Record transition
	transition := StateTransition{
		TicketID:    ticketID,
		FromState:   oldState,
		ToState:     StateConsumed,
		ValidatorID: validatorID,
		Timestamp:   ticket.Timestamp,
		VectorClock: ticket.VectorClock.Copy(),
	}
	sm.transitions = append(sm.transitions, transition)

	// Call handlers
	for _, handler := range sm.handlers {
		if err := handler(ticket, oldState, StateConsumed); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	return nil
}

// DisputeTicket marks a ticket as disputed
func (sm *StateMachine) DisputeTicket(ticketID string, validatorID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ticket, exists := sm.currentState[ticketID]
	if !exists {
		return fmt.Errorf("ticket %s not found", ticketID)
	}

	// Transition to DISPUTED
	oldState := ticket.State
	ticket.State = StateDisputed
	ticket.Timestamp = time.Now().Unix()

	// Increment vector clock
	sm.vectorClock.Increment()
	ticket.VectorClock = sm.vectorClock.Copy()

	// Record transition
	transition := StateTransition{
		TicketID:    ticketID,
		FromState:   oldState,
		ToState:     StateDisputed,
		ValidatorID: validatorID,
		Timestamp:   ticket.Timestamp,
		VectorClock: ticket.VectorClock.Copy(),
	}
	sm.transitions = append(sm.transitions, transition)

	// Call handlers
	for _, handler := range sm.handlers {
		if err := handler(ticket, oldState, StateDisputed); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	return nil
}

// GetTicket returns the current state of a ticket
func (sm *StateMachine) GetTicket(ticketID string) (*Ticket, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ticket, exists := sm.currentState[ticketID]
	if !exists {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}

	// Return a copy
	return &Ticket{
		ID:          ticket.ID,
		State:       ticket.State,
		Data:        ticket.Data,
		ValidatorID: ticket.ValidatorID,
		Timestamp:   ticket.Timestamp,
		VectorClock: ticket.VectorClock.Copy(),
		Metadata:    copyMetadata(ticket.Metadata),
	}, nil
}

// GetAllTickets returns all tickets
func (sm *StateMachine) GetAllTickets() []*Ticket {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tickets := make([]*Ticket, 0, len(sm.currentState))
	for _, ticket := range sm.currentState {
		tickets = append(tickets, &Ticket{
			ID:          ticket.ID,
			State:       ticket.State,
			Data:        ticket.Data,
			ValidatorID: ticket.ValidatorID,
			Timestamp:   ticket.Timestamp,
			VectorClock: ticket.VectorClock.Copy(),
			Metadata:    copyMetadata(ticket.Metadata),
		})
	}
	return tickets
}

// GetTransitions returns all state transitions
func (sm *StateMachine) GetTransitions() []StateTransition {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	transitions := make([]StateTransition, len(sm.transitions))
	copy(transitions, sm.transitions)
	return transitions
}

// MergeState merges state from another node
func (sm *StateMachine) MergeState(tickets []*Ticket) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, remoteTicket := range tickets {
		localTicket, exists := sm.currentState[remoteTicket.ID]

		if !exists {
			// New ticket, add it
			sm.currentState[remoteTicket.ID] = &Ticket{
				ID:          remoteTicket.ID,
				State:       remoteTicket.State,
				Data:        remoteTicket.Data,
				ValidatorID: remoteTicket.ValidatorID,
				Timestamp:   remoteTicket.Timestamp,
				VectorClock: remoteTicket.VectorClock.Copy(),
				Metadata:    copyMetadata(remoteTicket.Metadata),
			}
			continue
		}

		// Compare vector clocks to determine which state is newer
		comparison := localTicket.VectorClock.Compare(remoteTicket.VectorClock)

		switch comparison {
		case Concurrent:
			// Conflict resolution: use ticket with higher timestamp
			if remoteTicket.Timestamp > localTicket.Timestamp {
				localTicket.State = remoteTicket.State
				localTicket.ValidatorID = remoteTicket.ValidatorID
				localTicket.Timestamp = remoteTicket.Timestamp
				localTicket.VectorClock = remoteTicket.VectorClock.Copy()
			}
		case Before:
			// Remote is newer, update local
			localTicket.State = remoteTicket.State
			localTicket.ValidatorID = remoteTicket.ValidatorID
			localTicket.Timestamp = remoteTicket.Timestamp
			localTicket.VectorClock = remoteTicket.VectorClock.Copy()
		case After, Equal:
			// Local is newer or equal, keep local state
			continue
		}
	}

	// Merge vector clocks
	for _, remoteTicket := range tickets {
		sm.vectorClock.Merge(remoteTicket.VectorClock)
	}

	return nil
}

// RegisterHandler registers a state change handler
func (sm *StateMachine) RegisterHandler(handler StateChangeHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.handlers = append(sm.handlers, handler)
}

// GetVectorClock returns a copy of the current vector clock
func (sm *StateMachine) GetVectorClock() *VectorClock {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.vectorClock.Copy()
}

// Helper functions

func copyMetadata(metadata map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range metadata {
		copy[k] = v
	}
	return copy
}
