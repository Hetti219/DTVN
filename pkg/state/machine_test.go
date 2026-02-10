package state

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// issueAndValidate is a test helper that issues a ticket then validates it.
func issueAndValidate(sm *StateMachine, ticketID, validatorID string, data []byte) error {
	if err := sm.IssueTicket(ticketID, data); err != nil {
		return err
	}
	return sm.ValidateTicket(ticketID, validatorID, data)
}

// TestNewStateMachine tests state machine creation
func TestNewStateMachine(t *testing.T) {
	t.Run("CreateStateMachine", func(t *testing.T) {
		sm := NewStateMachine("node1")
		require.NotNil(t, sm)
		assert.Equal(t, "node1", sm.nodeID)
		assert.NotNil(t, sm.currentState)
		assert.NotNil(t, sm.transitions)
		assert.NotNil(t, sm.vectorClock)
	})

	t.Run("CreateMultipleStateMachines", func(t *testing.T) {
		sm1 := NewStateMachine("node1")
		sm2 := NewStateMachine("node2")

		assert.Equal(t, "node1", sm1.nodeID)
		assert.Equal(t, "node2", sm2.nodeID)
	})
}

// TestIssueTicket tests ticket issuance
func TestIssueTicket(t *testing.T) {
	t.Run("IssueNewTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.IssueTicket("ticket-001", []byte("test-data"))
		require.NoError(t, err)

		ticket, err := sm.GetTicket("ticket-001")
		require.NoError(t, err)
		assert.Equal(t, StateIssued, ticket.State)
		assert.Equal(t, "", ticket.ValidatorID)
	})

	t.Run("CannotIssueDuplicate", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.IssueTicket("ticket-dup", []byte("test"))
		require.NoError(t, err)

		err = sm.IssueTicket("ticket-dup", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

// TestSeedTickets tests bulk ticket seeding
func TestSeedTickets(t *testing.T) {
	t.Run("SeedMultipleTickets", func(t *testing.T) {
		sm := NewStateMachine("node1")

		tickets := make([]*Ticket, 10)
		for i := 0; i < 10; i++ {
			tickets[i] = &Ticket{
				ID:        fmt.Sprintf("TICKET-%03d", i+1),
				State:     StateIssued,
				Data:      []byte("test"),
				Timestamp: time.Now().Unix(),
				Metadata:  map[string]string{"source": "seed"},
			}
		}

		seeded, err := sm.SeedTickets(tickets)
		require.NoError(t, err)
		assert.Equal(t, 10, seeded)
		assert.Len(t, sm.GetAllTickets(), 10)
	})

	t.Run("SeedSkipsDuplicates", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("TICKET-001", []byte("existing"))

		tickets := []*Ticket{
			{ID: "TICKET-001", State: StateIssued, Data: []byte("dup"), Timestamp: time.Now().Unix(), Metadata: map[string]string{}},
			{ID: "TICKET-002", State: StateIssued, Data: []byte("new"), Timestamp: time.Now().Unix(), Metadata: map[string]string{}},
		}

		seeded, err := sm.SeedTickets(tickets)
		require.NoError(t, err)
		assert.Equal(t, 1, seeded) // Only TICKET-002 is new
		assert.Len(t, sm.GetAllTickets(), 2)
	})
}

// TestValidateTicket tests ticket validation
func TestValidateTicket(t *testing.T) {
	t.Run("ValidateIssuedTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("ticket-001", []byte("test-data"))
		err := sm.ValidateTicket("ticket-001", "validator-1", []byte("test-data"))
		require.NoError(t, err)

		ticket, err := sm.GetTicket("ticket-001")
		require.NoError(t, err)
		assert.Equal(t, StateValidated, ticket.State)
		assert.Equal(t, "validator-1", ticket.ValidatorID)
	})

	t.Run("CannotValidateNonExistentTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.ValidateTicket("fake-ticket", "validator-1", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("ValidateMultipleTickets", func(t *testing.T) {
		sm := NewStateMachine("node1")

		for i := 1; i <= 5; i++ {
			ticketID := "ticket-" + string(rune('0'+i))
			sm.IssueTicket(ticketID, []byte("test"))
			err := sm.ValidateTicket(ticketID, "validator-1", []byte("test"))
			require.NoError(t, err)
		}

		tickets := sm.GetAllTickets()
		assert.Len(t, tickets, 5)
	})

	t.Run("CannotValidateTwice", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("ticket-double", []byte("test"))
		err := sm.ValidateTicket("ticket-double", "validator-1", []byte("test"))
		require.NoError(t, err)

		// Try to validate again
		err = sm.ValidateTicket("ticket-double", "validator-1", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already validated")
	})

	t.Run("CannotValidateConsumedTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		// Issue, validate, then consume
		sm.IssueTicket("ticket-consumed", []byte("test"))
		sm.ValidateTicket("ticket-consumed", "validator-1", []byte("test"))
		sm.ConsumeTicket("ticket-consumed", "validator-1")

		// Try to validate consumed ticket
		err := sm.ValidateTicket("ticket-consumed", "validator-1", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already consumed")
	})

	t.Run("CannotValidateDisputedTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("ticket-disputed", []byte("test"))
		sm.ValidateTicket("ticket-disputed", "validator-1", []byte("test"))
		sm.DisputeTicket("ticket-disputed", "validator-1")

		// Try to validate disputed ticket
		err := sm.ValidateTicket("ticket-disputed", "validator-1", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "disputed")
	})

	t.Run("ValidateUpdatesVectorClock", func(t *testing.T) {
		sm := NewStateMachine("node1")

		initialClock := sm.GetVectorClock().Get("node1")

		sm.IssueTicket("ticket-clock", []byte("test"))
		err := sm.ValidateTicket("ticket-clock", "validator-1", []byte("test"))
		require.NoError(t, err)

		ticket, _ := sm.GetTicket("ticket-clock")
		assert.Greater(t, ticket.VectorClock.Get("node1"), initialClock)
	})

	t.Run("ValidateRecordsTransition", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("ticket-transition", []byte("test"))
		err := sm.ValidateTicket("ticket-transition", "validator-1", []byte("test"))
		require.NoError(t, err)

		transitions := sm.GetTransitions()
		assert.Len(t, transitions, 1)
		assert.Equal(t, "ticket-transition", transitions[0].TicketID)
		assert.Equal(t, StateIssued, transitions[0].FromState)
		assert.Equal(t, StateValidated, transitions[0].ToState)
	})
}

// TestConsumeTicket tests ticket consumption
func TestConsumeTicket(t *testing.T) {
	t.Run("ConsumeValidatedTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-consume", "validator-1", []byte("test"))

		err := sm.ConsumeTicket("ticket-consume", "validator-1")
		require.NoError(t, err)

		ticket, err := sm.GetTicket("ticket-consume")
		require.NoError(t, err)
		assert.Equal(t, StateConsumed, ticket.State)
	})

	t.Run("CannotConsumeNonExistentTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.ConsumeTicket("non-existent", "validator-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("CannotConsumeNonValidatedTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		// Issue ticket but don't validate it
		sm.IssueTicket("ticket-issued-only", []byte("test"))

		err := sm.ConsumeTicket("ticket-issued-only", "validator-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not validated")
	})

	t.Run("ConsumeUpdatesVectorClock", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-consume-clock", "validator-1", []byte("test"))

		ticket1, _ := sm.GetTicket("ticket-consume-clock")
		clockBefore := ticket1.VectorClock.Get("node1")

		sm.ConsumeTicket("ticket-consume-clock", "validator-1")

		ticket2, _ := sm.GetTicket("ticket-consume-clock")
		assert.Greater(t, ticket2.VectorClock.Get("node1"), clockBefore)
	})
}

// TestDisputeTicket tests ticket disputes
func TestDisputeTicket(t *testing.T) {
	t.Run("DisputeExistingTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-dispute", "validator-1", []byte("test"))

		err := sm.DisputeTicket("ticket-dispute", "validator-1")
		require.NoError(t, err)

		ticket, err := sm.GetTicket("ticket-dispute")
		require.NoError(t, err)
		assert.Equal(t, StateDisputed, ticket.State)
	})

	t.Run("CannotDisputeNonExistentTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.DisputeTicket("non-existent", "validator-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("DisputeUpdatesVectorClock", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-dispute-clock", "validator-1", []byte("test"))

		ticket1, _ := sm.GetTicket("ticket-dispute-clock")
		clockBefore := ticket1.VectorClock.Get("node1")

		sm.DisputeTicket("ticket-dispute-clock", "validator-1")

		ticket2, _ := sm.GetTicket("ticket-dispute-clock")
		assert.Greater(t, ticket2.VectorClock.Get("node1"), clockBefore)
	})
}

// TestGetTicket tests ticket retrieval
func TestGetTicket(t *testing.T) {
	t.Run("GetExistingTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-get", "validator-1", []byte("test-data"))

		ticket, err := sm.GetTicket("ticket-get")
		require.NoError(t, err)
		assert.Equal(t, "ticket-get", ticket.ID)
	})

	t.Run("GetNonExistentTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		ticket, err := sm.GetTicket("non-existent")
		assert.Error(t, err)
		assert.Nil(t, ticket)
	})

	t.Run("GetReturnsACopy", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-copy", "validator-1", []byte("test"))

		ticket, _ := sm.GetTicket("ticket-copy")
		ticket.State = StateConsumed // Modify the copy

		// Original should not be affected
		ticket2, _ := sm.GetTicket("ticket-copy")
		assert.Equal(t, StateValidated, ticket2.State)
	})
}

// TestGetAllTickets tests retrieving all tickets
func TestGetAllTickets(t *testing.T) {
	t.Run("GetAllTickets", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-1", "validator-1", []byte("test1"))
		issueAndValidate(sm, "ticket-2", "validator-1", []byte("test2"))
		issueAndValidate(sm, "ticket-3", "validator-1", []byte("test3"))

		tickets := sm.GetAllTickets()
		assert.Len(t, tickets, 3)
	})

	t.Run("GetAllTicketsWhenEmpty", func(t *testing.T) {
		sm := NewStateMachine("node1")

		tickets := sm.GetAllTickets()
		assert.Empty(t, tickets)
	})
}

// TestHasTicket tests ticket existence check
func TestHasTicket(t *testing.T) {
	t.Run("HasExistingTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("ticket-exists", []byte("test"))

		assert.True(t, sm.HasTicket("ticket-exists"))
		assert.False(t, sm.HasTicket("non-existent"))
	})
}

// TestStateChangeHandlers tests handler registration and execution
func TestStateChangeHandlers(t *testing.T) {
	t.Run("RegisterSingleHandler", func(t *testing.T) {
		sm := NewStateMachine("node1")

		called := false
		sm.RegisterHandler(func(ticket *Ticket, oldState, newState TicketState) error {
			called = true
			return nil
		})

		sm.IssueTicket("ticket-handler", []byte("test"))
		sm.ValidateTicket("ticket-handler", "validator-1", []byte("test"))
		assert.True(t, called)
	})

	t.Run("RegisterMultipleHandlers", func(t *testing.T) {
		sm := NewStateMachine("node1")

		callCount := 0
		for i := 0; i < 3; i++ {
			sm.RegisterHandler(func(ticket *Ticket, oldState, newState TicketState) error {
				callCount++
				return nil
			})
		}

		sm.IssueTicket("ticket-multi-handler", []byte("test"))
		sm.ValidateTicket("ticket-multi-handler", "validator-1", []byte("test"))
		assert.Equal(t, 3, callCount)
	})

	t.Run("HandlerReceivesCorrectStates", func(t *testing.T) {
		sm := NewStateMachine("node1")

		var capturedOldState, capturedNewState TicketState
		sm.RegisterHandler(func(ticket *Ticket, oldState, newState TicketState) error {
			capturedOldState = oldState
			capturedNewState = newState
			return nil
		})

		sm.IssueTicket("ticket-states", []byte("test"))
		sm.ValidateTicket("ticket-states", "validator-1", []byte("test"))

		assert.Equal(t, StateIssued, capturedOldState)
		assert.Equal(t, StateValidated, capturedNewState)
	})
}

// TestMergeState tests state merging
func TestMergeState(t *testing.T) {
	t.Run("MergeNewTickets", func(t *testing.T) {
		sm1 := NewStateMachine("node1")
		sm2 := NewStateMachine("node2")

		issueAndValidate(sm2, "ticket-remote", "validator-2", []byte("test"))

		remoteTickets := sm2.GetAllTickets()
		err := sm1.MergeState(remoteTickets)
		require.NoError(t, err)

		assert.True(t, sm1.HasTicket("ticket-remote"))
	})

	t.Run("MergeWithNewerState", func(t *testing.T) {
		sm1 := NewStateMachine("node1")
		sm2 := NewStateMachine("node2")

		// Both create same ticket
		issueAndValidate(sm1, "ticket-conflict", "validator-1", []byte("test"))
		time.Sleep(10 * time.Millisecond)
		issueAndValidate(sm2, "ticket-conflict", "validator-2", []byte("test"))

		// sm2 has newer timestamp
		ticket2, _ := sm2.GetTicket("ticket-conflict")
		ticket2.Timestamp = time.Now().Unix() + 1000

		sm2.mu.Lock()
		sm2.currentState["ticket-conflict"] = ticket2
		sm2.mu.Unlock()

		remoteTickets := sm2.GetAllTickets()
		err := sm1.MergeState(remoteTickets)
		require.NoError(t, err)

		// Should adopt newer state
		ticket1, _ := sm1.GetTicket("ticket-conflict")
		assert.Equal(t, "validator-2", ticket1.ValidatorID)
	})

	t.Run("MergeWithOlderState", func(t *testing.T) {
		sm1 := NewStateMachine("node1")
		sm2 := NewStateMachine("node2")

		issueAndValidate(sm1, "ticket-newer", "validator-1", []byte("test"))
		time.Sleep(10 * time.Millisecond)

		// Create older version in sm2
		sm2.mu.Lock()
		sm2.currentState["ticket-newer"] = &Ticket{
			ID:          "ticket-newer",
			State:       StateIssued,
			ValidatorID: "validator-2",
			Timestamp:   time.Now().Unix() - 1000,
			VectorClock: sm2.vectorClock.Copy(),
		}
		sm2.mu.Unlock()

		remoteTickets := sm2.GetAllTickets()
		err := sm1.MergeState(remoteTickets)
		require.NoError(t, err)

		// Should keep newer local state
		ticket1, _ := sm1.GetTicket("ticket-newer")
		assert.Equal(t, "validator-1", ticket1.ValidatorID)
	})
}

// TestSyncTicket tests ticket synchronization
func TestSyncTicket(t *testing.T) {
	t.Run("SyncNewTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.SyncTicket("ticket-sync", "validator-remote", []byte("synced-data"))
		require.NoError(t, err)

		ticket, err := sm.GetTicket("ticket-sync")
		require.NoError(t, err)
		assert.Equal(t, StateValidated, ticket.State)
		assert.Equal(t, "validator-remote", ticket.ValidatorID)
		assert.Equal(t, "true", ticket.Metadata["synced"])
	})

	t.Run("CannotSyncExistingTicket", func(t *testing.T) {
		sm := NewStateMachine("node1")

		sm.IssueTicket("ticket-exists", []byte("test"))

		err := sm.SyncTicket("ticket-exists", "validator-remote", []byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("SyncRecordsTransition", func(t *testing.T) {
		sm := NewStateMachine("node1")

		err := sm.SyncTicket("ticket-sync-trans", "validator-remote", []byte("test"))
		require.NoError(t, err)

		transitions := sm.GetTransitions()
		assert.NotEmpty(t, transitions)
	})
}

// TestVectorClockIntegration tests vector clock integration
func TestVectorClockIntegration(t *testing.T) {
	t.Run("VectorClockIncrementsOnValidate", func(t *testing.T) {
		sm := NewStateMachine("node1")

		initialClock := sm.GetVectorClock().Get("node1")

		sm.IssueTicket("ticket-vc1", []byte("test"))
		sm.ValidateTicket("ticket-vc1", "validator-1", []byte("test"))

		currentClock := sm.GetVectorClock().Get("node1")
		assert.Greater(t, currentClock, initialClock)
	})

	t.Run("VectorClockIncrementsOnConsume", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-vc2", "validator-1", []byte("test"))
		clockAfterValidate := sm.GetVectorClock().Get("node1")

		sm.ConsumeTicket("ticket-vc2", "validator-1")
		clockAfterConsume := sm.GetVectorClock().Get("node1")

		assert.Greater(t, clockAfterConsume, clockAfterValidate)
	})

	t.Run("VectorClockIncrementsOnDispute", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-vc3", "validator-1", []byte("test"))
		clockAfterValidate := sm.GetVectorClock().Get("node1")

		sm.DisputeTicket("ticket-vc3", "validator-1")
		clockAfterDispute := sm.GetVectorClock().Get("node1")

		assert.Greater(t, clockAfterDispute, clockAfterValidate)
	})
}

// TestConcurrentOperations tests thread safety
func TestStateMachineConcurrentOperations(t *testing.T) {
	t.Run("ConcurrentValidations", func(t *testing.T) {
		sm := NewStateMachine("node1")

		// Pre-issue all tickets
		for i := 0; i < 10; i++ {
			ticketID := "concurrent-" + string(rune('0'+i))
			sm.IssueTicket(ticketID, []byte("test"))
		}

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				ticketID := "concurrent-" + string(rune('0'+id))
				err := sm.ValidateTicket(ticketID, "validator-1", []byte("test"))
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		tickets := sm.GetAllTickets()
		assert.Len(t, tickets, 10)
	})

	t.Run("ConcurrentReads", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "shared-ticket", "validator-1", []byte("test"))

		var wg sync.WaitGroup
		wg.Add(20)

		for i := 0; i < 20; i++ {
			go func() {
				defer wg.Done()
				ticket, err := sm.GetTicket("shared-ticket")
				assert.NoError(t, err)
				assert.NotNil(t, ticket)
			}()
		}

		wg.Wait()
	})

	t.Run("ConcurrentHandlerRegistration", func(t *testing.T) {
		sm := NewStateMachine("node1")

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				sm.RegisterHandler(func(ticket *Ticket, oldState, newState TicketState) error {
					return nil
				})
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		sm.mu.RLock()
		handlerCount := len(sm.handlers)
		sm.mu.RUnlock()

		assert.Equal(t, 10, handlerCount)
	})
}

// TestStateTransitions tests state transition tracking
func TestStateTransitions(t *testing.T) {
	t.Run("RecordsAllTransitions", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-trans", "validator-1", []byte("test"))
		sm.ConsumeTicket("ticket-trans", "validator-1")

		transitions := sm.GetTransitions()
		assert.Len(t, transitions, 2)

		assert.Equal(t, StateIssued, transitions[0].FromState)
		assert.Equal(t, StateValidated, transitions[0].ToState)

		assert.Equal(t, StateValidated, transitions[1].FromState)
		assert.Equal(t, StateConsumed, transitions[1].ToState)
	})

	t.Run("TransitionsIncludeVectorClock", func(t *testing.T) {
		sm := NewStateMachine("node1")

		issueAndValidate(sm, "ticket-trans-vc", "validator-1", []byte("test"))

		transitions := sm.GetTransitions()
		assert.Len(t, transitions, 1)
		assert.NotNil(t, transitions[0].VectorClock)
	})
}

// TestPublisher tests state update publisher
func TestPublisher(t *testing.T) {
	t.Run("RegisterPublisher", func(t *testing.T) {
		sm := NewStateMachine("node1")

		var published atomic.Bool
		sm.RegisterPublisher(func(ticketID string, state TicketState, validatorID string, timestamp int64) error {
			published.Store(true)
			return nil
		})

		sm.IssueTicket("ticket-publish", []byte("test"))
		sm.ValidateTicket("ticket-publish", "validator-1", []byte("test"))

		// Give goroutine time to execute
		time.Sleep(50 * time.Millisecond)

		assert.True(t, published.Load())
	})
}

// TestMetadataCopy tests metadata copying
func TestMetadataCopy(t *testing.T) {
	t.Run("CopyMetadata", func(t *testing.T) {
		original := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		copied := copyMetadata(original)

		assert.Equal(t, original, copied)

		// Modify copy
		copied["key3"] = "value3"

		// Original should not be affected
		assert.NotEqual(t, original, copied)
		assert.NotContains(t, original, "key3")
	})
}
