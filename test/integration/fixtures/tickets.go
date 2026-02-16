package fixtures

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// TestTicket represents a test ticket
type TestTicket struct {
	ID       string
	Data     []byte
	EventID  string
	Metadata map[string]string
}

// GenerateTicket generates a test ticket with a unique ID
func GenerateTicket(prefix string, index int) *TestTicket {
	ticketID := fmt.Sprintf("%s-%d-%s", prefix, index, generateRandomID())

	return &TestTicket{
		ID:      ticketID,
		Data:    []byte(fmt.Sprintf("Test ticket data for %s", ticketID)),
		EventID: fmt.Sprintf("EVENT-%d", index%10),
		Metadata: map[string]string{
			"type":     "integration_test",
			"index":    fmt.Sprintf("%d", index),
			"event_id": fmt.Sprintf("EVENT-%d", index%10),
		},
	}
}

// GenerateTickets generates multiple test tickets
func GenerateTickets(prefix string, count int) []*TestTicket {
	tickets := make([]*TestTicket, count)
	for i := 0; i < count; i++ {
		tickets[i] = GenerateTicket(prefix, i)
	}
	return tickets
}

// GenerateDuplicateTicket creates a ticket with the same ID as an existing ticket
func GenerateDuplicateTicket(original *TestTicket) *TestTicket {
	return &TestTicket{
		ID:      original.ID,
		Data:    []byte(fmt.Sprintf("Duplicate data for %s", original.ID)),
		EventID: original.EventID,
		Metadata: map[string]string{
			"type":     "duplicate_test",
			"original": "false",
			"event_id": original.EventID,
		},
	}
}

// generateRandomID generates a random hex string
func generateRandomID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
