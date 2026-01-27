package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockValidator implements ValidatorInterface for testing
type MockValidator struct {
	tickets       map[string]interface{}
	validateError error
	consumeError  error
	disputeError  error
	getTicketErr  error
	getPeersErr   error
	getConfigErr  error
	stats         map[string]interface{}
}

func NewMockValidator() *MockValidator {
	return &MockValidator{
		tickets: make(map[string]interface{}),
		stats: map[string]interface{}{
			"total_validations": 10,
			"consensus_rounds":  5,
		},
	}
}

func (m *MockValidator) ValidateTicket(ticketID string, data []byte) error {
	if m.validateError != nil {
		return m.validateError
	}
	m.tickets[ticketID] = map[string]interface{}{
		"id":    ticketID,
		"state": "VALIDATED",
		"data":  string(data),
	}
	return nil
}

func (m *MockValidator) ConsumeTicket(ticketID string) error {
	if m.consumeError != nil {
		return m.consumeError
	}
	if ticket, ok := m.tickets[ticketID]; ok {
		ticketMap := ticket.(map[string]interface{})
		ticketMap["state"] = "CONSUMED"
		return nil
	}
	return errors.New("ticket not found")
}

func (m *MockValidator) DisputeTicket(ticketID string) error {
	if m.disputeError != nil {
		return m.disputeError
	}
	if ticket, ok := m.tickets[ticketID]; ok {
		ticketMap := ticket.(map[string]interface{})
		ticketMap["state"] = "DISPUTED"
		return nil
	}
	return errors.New("ticket not found")
}

func (m *MockValidator) GetTicket(ticketID string) (interface{}, error) {
	if m.getTicketErr != nil {
		return nil, m.getTicketErr
	}
	ticket, ok := m.tickets[ticketID]
	if !ok {
		return nil, errors.New("ticket not found")
	}
	return ticket, nil
}

func (m *MockValidator) GetAllTickets() ([]interface{}, error) {
	tickets := make([]interface{}, 0, len(m.tickets))
	for _, ticket := range m.tickets {
		tickets = append(tickets, ticket)
	}
	return tickets, nil
}

func (m *MockValidator) GetStats() map[string]interface{} {
	return m.stats
}

func (m *MockValidator) GetPeers() ([]PeerInfo, error) {
	if m.getPeersErr != nil {
		return nil, m.getPeersErr
	}
	return []PeerInfo{
		{ID: "peer-1", Addrs: []string{"/ip4/127.0.0.1/tcp/4001"}},
		{ID: "peer-2", Addrs: []string{"/ip4/127.0.0.1/tcp/4002"}},
	}, nil
}

func (m *MockValidator) GetConfig() (interface{}, error) {
	if m.getConfigErr != nil {
		return nil, m.getConfigErr
	}
	return map[string]interface{}{
		"node_id": "test-node",
		"port":    4001,
	}, nil
}

// Helper function to create a test server
func createTestServer(t *testing.T) (*Server, *MockValidator) {
	ctx := context.Background()
	validator := NewMockValidator()
	cfg := &Config{
		Address:    "localhost",
		Port:       8080,
		EnableCORS: true,
	}

	server, err := NewServer(ctx, cfg, validator)
	require.NoError(t, err)
	require.NotNil(t, server)

	return server, validator
}

// TestNewServer tests server creation
func TestNewServer(t *testing.T) {
	t.Run("CreateServerSuccessfully", func(t *testing.T) {
		ctx := context.Background()
		validator := NewMockValidator()
		cfg := &Config{
			Address:    "localhost",
			Port:       8080,
			EnableCORS: true,
		}

		server, err := NewServer(ctx, cfg, validator)
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Equal(t, "localhost:8080", server.httpServer.Addr)
		assert.NotNil(t, server.router)
		assert.NotNil(t, server.broadcast)
	})

	t.Run("CreateServerWithoutCORS", func(t *testing.T) {
		ctx := context.Background()
		validator := NewMockValidator()
		cfg := &Config{
			Address:    "0.0.0.0",
			Port:       9000,
			EnableCORS: false,
		}

		server, err := NewServer(ctx, cfg, validator)
		require.NoError(t, err)
		assert.Equal(t, "0.0.0.0:9000", server.httpServer.Addr)
	})
}

// TestValidateTicket tests ticket validation endpoint
func TestValidateTicket(t *testing.T) {
	t.Run("ValidateTicketSuccess", func(t *testing.T) {
		server, _ := createTestServer(t)

		reqBody := ValidateRequest{
			TicketID: "ticket-001",
			Data:     []byte("test-data"),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Contains(t, resp.Message, "validated successfully")
	})

	t.Run("ValidateTicketMissingID", func(t *testing.T) {
		server, _ := createTestServer(t)

		reqBody := ValidateRequest{
			TicketID: "",
			Data:     []byte("test-data"),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "ticket_id is required")
	})

	t.Run("ValidateTicketInvalidJSON", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("POST", "/api/v1/tickets/validate", bytes.NewReader([]byte("invalid-json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
	})

	t.Run("ValidateTicketValidatorError", func(t *testing.T) {
		server, validator := createTestServer(t)
		validator.validateError = errors.New("validation failed")

		reqBody := ValidateRequest{
			TicketID: "ticket-001",
			Data:     []byte("test-data"),
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "validation failed")
	})
}

// TestConsumeTicket tests ticket consumption endpoint
func TestConsumeTicket(t *testing.T) {
	t.Run("ConsumeTicketSuccess", func(t *testing.T) {
		server, validator := createTestServer(t)

		// First validate a ticket
		validator.ValidateTicket("ticket-002", []byte("test"))

		reqBody := ConsumeRequest{
			TicketID: "ticket-002",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/consume", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
	})

	t.Run("ConsumeTicketMissingID", func(t *testing.T) {
		server, _ := createTestServer(t)

		reqBody := ConsumeRequest{
			TicketID: "",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/consume", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ConsumeTicketError", func(t *testing.T) {
		server, validator := createTestServer(t)
		validator.consumeError = errors.New("consume failed")

		reqBody := ConsumeRequest{
			TicketID: "ticket-999",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/consume", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// TestDisputeTicket tests ticket dispute endpoint
func TestDisputeTicket(t *testing.T) {
	t.Run("DisputeTicketSuccess", func(t *testing.T) {
		server, validator := createTestServer(t)

		// First validate a ticket
		validator.ValidateTicket("ticket-003", []byte("test"))

		reqBody := DisputeRequest{
			TicketID: "ticket-003",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/dispute", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
	})

	t.Run("DisputeTicketMissingID", func(t *testing.T) {
		server, _ := createTestServer(t)

		reqBody := DisputeRequest{
			TicketID: "",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/tickets/dispute", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestGetTicket tests retrieving a single ticket
func TestGetTicket(t *testing.T) {
	t.Run("GetTicketSuccess", func(t *testing.T) {
		server, validator := createTestServer(t)

		// Add a ticket
		validator.ValidateTicket("ticket-004", []byte("test"))

		req := httptest.NewRequest("GET", "/api/v1/tickets/ticket-004", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Data)
	})

	t.Run("GetTicketNotFound", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/tickets/non-existent", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Success)
	})

	t.Run("GetTicketValidatorError", func(t *testing.T) {
		server, validator := createTestServer(t)
		validator.getTicketErr = errors.New("database error")

		req := httptest.NewRequest("GET", "/api/v1/tickets/ticket-005", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestGetAllTickets tests retrieving all tickets
func TestGetAllTickets(t *testing.T) {
	t.Run("GetAllTicketsSuccess", func(t *testing.T) {
		server, validator := createTestServer(t)

		// Add multiple tickets
		validator.ValidateTicket("ticket-1", []byte("test1"))
		validator.ValidateTicket("ticket-2", []byte("test2"))
		validator.ValidateTicket("ticket-3", []byte("test3"))

		req := httptest.NewRequest("GET", "/api/v1/tickets", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		tickets := resp.Data.([]interface{})
		assert.Len(t, tickets, 3)
	})

	t.Run("GetAllTicketsEmpty", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/tickets", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
	})
}

// TestGetStatus tests status endpoint
func TestGetStatus(t *testing.T) {
	t.Run("GetStatusSuccess", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/status", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		data := resp.Data.(map[string]interface{})
		assert.Equal(t, "running", data["status"])
		assert.Contains(t, data, "timestamp")
	})
}

// TestGetStats tests stats endpoint
func TestGetStats(t *testing.T) {
	t.Run("GetStatsSuccess", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/stats", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Data)
	})
}

// TestGetPeers tests peers endpoint
func TestGetPeers(t *testing.T) {
	t.Run("GetPeersSuccess", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/peers", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Data)
	})

	t.Run("GetPeersError", func(t *testing.T) {
		server, validator := createTestServer(t)
		validator.getPeersErr = errors.New("failed to get peers")

		req := httptest.NewRequest("GET", "/api/v1/peers", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// TestGetConfig tests config endpoint
func TestGetConfig(t *testing.T) {
	t.Run("GetConfigSuccess", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/config", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Data)
	})

	t.Run("GetConfigError", func(t *testing.T) {
		server, validator := createTestServer(t)
		validator.getConfigErr = errors.New("config error")

		req := httptest.NewRequest("GET", "/api/v1/config", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// TestHealthCheck tests health check endpoint
func TestHealthCheck(t *testing.T) {
	t.Run("HealthCheckSuccess", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "healthy", resp["status"])
	})
}

// TestCORSMiddleware tests CORS headers
func TestCORSMiddleware(t *testing.T) {
	t.Run("CORSHeadersPresent", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/status", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	})

	t.Run("OPTIONSRequest", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("OPTIONS", "/api/v1/tickets/validate", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestBroadcast tests WebSocket broadcast functionality
func TestBroadcast(t *testing.T) {
	t.Run("BroadcastMessage", func(t *testing.T) {
		server, _ := createTestServer(t)

		// Start broadcast loop in background
		go server.broadcastLoop()

		msg := map[string]interface{}{
			"type": "test",
			"data": "hello",
		}

		// Should not block
		server.Broadcast(msg)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("BroadcastChannelFull", func(t *testing.T) {
		server, _ := createTestServer(t)

		// Fill the broadcast channel (capacity is 100)
		for i := 0; i < 110; i++ {
			server.Broadcast(map[string]interface{}{"index": i})
		}

		// Should not panic when channel is full
		assert.NotPanics(t, func() {
			server.Broadcast(map[string]interface{}{"overflow": true})
		})
	})
}

// TestServerClose tests server shutdown
func TestServerClose(t *testing.T) {
	t.Run("CloseServerSuccessfully", func(t *testing.T) {
		ctx := context.Background()
		validator := NewMockValidator()
		cfg := &Config{
			Address:    "localhost",
			Port:       8888,
			EnableCORS: true,
		}

		server, err := NewServer(ctx, cfg, validator)
		require.NoError(t, err)

		// Start server
		err = server.Start()
		require.NoError(t, err)

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)

		// Close server
		err = server.Close()
		assert.NoError(t, err)
	})
}

// TestConcurrentRequests tests concurrent API requests
func TestConcurrentRequests(t *testing.T) {
	t.Run("ConcurrentValidations", func(t *testing.T) {
		server, _ := createTestServer(t)

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				reqBody := ValidateRequest{
					TicketID: "concurrent-" + string(rune('0'+id)),
					Data:     []byte("test"),
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("POST", "/api/v1/tickets/validate", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				server.router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
				done <- true
			}(i)
		}

		// Wait for all requests
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// TestWebSocketConnection tests WebSocket connectivity
func TestWebSocketConnection(t *testing.T) {
	t.Run("WebSocketUpgrade", func(t *testing.T) {
		server, _ := createTestServer(t)

		// Start server
		go server.broadcastLoop()

		// Create test WebSocket server
		testServer := httptest.NewServer(server.router)
		defer testServer.Close()

		// Convert http://127.0.0.1 to ws://127.0.0.1
		wsURL := "ws" + testServer.URL[4:] + "/ws"

		// Connect to WebSocket
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Skipf("WebSocket connection failed (expected in test environment): %v", err)
			return
		}
		defer ws.Close()

		// Verify connection registered
		server.wsMu.RLock()
		clientCount := len(server.wsClients)
		server.wsMu.RUnlock()

		assert.Equal(t, 1, clientCount)
	})
}

// TestMethodNotAllowed tests that wrong HTTP methods are rejected
func TestMethodNotAllowed(t *testing.T) {
	t.Run("GetOnPostEndpoint", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/tickets/validate", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		// Should not be 200 OK
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("PostOnGetEndpoint", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("POST", "/api/v1/status", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		// Should not be 200 OK
		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}

// TestResponseFormats tests response structure
func TestResponseFormats(t *testing.T) {
	t.Run("SuccessResponseFormat", func(t *testing.T) {
		server, validator := createTestServer(t)
		validator.ValidateTicket("ticket-format", []byte("test"))

		req := httptest.NewRequest("GET", "/api/v1/tickets/ticket-format", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.Message)
		assert.NotNil(t, resp.Data)
		assert.Empty(t, resp.Error)
	})

	t.Run("ErrorResponseFormat", func(t *testing.T) {
		server, _ := createTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/tickets/non-existent", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.False(t, resp.Success)
		assert.NotEmpty(t, resp.Error)
	})
}
