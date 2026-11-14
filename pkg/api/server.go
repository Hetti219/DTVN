package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// maxRequestBodySize limits the size of incoming JSON request bodies (1 MB).
// Prevents clients from sending excessively large payloads that waste memory.
const maxRequestBodySize = 1 << 20

// JSON keys used in the broadcast payloads and status maps produced by the
// API server.
const (
	jsonKeyType      = "type"
	jsonKeyTicketID  = "ticket_id"
	jsonKeyTimestamp = "timestamp"
)

// Server represents the API server
type Server struct {
	router     *mux.Router
	httpServer *http.Server
	validator  ValidatorInterface
	wsClients  map[*websocket.Conn]bool
	wsUpgrader websocket.Upgrader
	wsMu       sync.RWMutex
	broadcast  chan interface{}
	ctx        context.Context
	cancel     context.CancelFunc
	apiKey     string
}

// ValidatorInterface defines the interface for validator operations
type ValidatorInterface interface {
	ValidateTicket(ticketID string, data []byte) error
	ConsumeTicket(ticketID string) error
	DisputeTicket(ticketID string) error
	SeedTickets() (int, error)
	GetTicket(ticketID string) (interface{}, error)
	GetAllTickets() ([]interface{}, error)
	GetStats() map[string]interface{}
	GetPeers() ([]PeerInfo, error)
	GetConfig() (interface{}, error)
	GetConsensusLogs() ([]interface{}, error)
	GetNodeCryptoInfo() map[string]interface{}
	GetStorageEntries() map[string]interface{}
}

// Config holds API server configuration
type Config struct {
	Address    string
	Port       int
	EnableCORS bool
	APIKey     string // Optional API key for authentication (empty = no auth)
}

// Request/Response structures

// ValidateRequest is the request body of the validate endpoint. The optional
// data field is carried as base64 and is rejected when it is malformed.
type ValidateRequest struct {
	TicketID string `json:"ticket_id"`
	Data     []byte `json:"data"`
}

// ConsumeRequest is the request body of the consume endpoint. Only the
// ticket identifier is read; any other field sent by a client is ignored.
type ConsumeRequest struct {
	TicketID string `json:"ticket_id"`
}

// DisputeRequest is the request body of the dispute endpoint. Only the
// ticket identifier is read; any other field sent by a client is ignored.
type DisputeRequest struct {
	TicketID string `json:"ticket_id"`
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// NewServer creates a new API server
func NewServer(ctx context.Context, cfg *Config, validator ValidatorInterface) (*Server, error) {
	serverCtx, cancel := context.WithCancel(ctx)

	s := &Server{
		router:    mux.NewRouter(),
		validator: validator,
		wsClients: make(map[*websocket.Conn]bool),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: checkWSOrigin,
		},
		broadcast: make(chan interface{}, 100),
		ctx:       serverCtx,
		cancel:    cancel,
		apiKey:    cfg.APIKey,
	}

	// Setup routes
	s.setupRoutes()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Address, cfg.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second, // Must exceed consensus timeout (20s) for non-primary forwarding
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// API v1 routes
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Ticket operations
	api.HandleFunc("/tickets/validate", s.handleValidateTicket).Methods("POST")
	api.HandleFunc("/tickets/consume", s.handleConsumeTicket).Methods("POST")
	api.HandleFunc("/tickets/dispute", s.handleDisputeTicket).Methods("POST")
	api.HandleFunc("/tickets/seed", s.handleSeedTickets).Methods("POST")
	api.HandleFunc("/tickets/{id}", s.handleGetTicket).Methods("GET")
	api.HandleFunc("/tickets", s.handleGetAllTickets).Methods("GET")

	// Node status
	api.HandleFunc("/status", s.handleGetStatus).Methods("GET")
	api.HandleFunc("/stats", s.handleGetStats).Methods("GET")

	// Peer and config information
	api.HandleFunc("/peers", s.handleGetPeers).Methods("GET")
	api.HandleFunc("/config", s.handleGetConfig).Methods("GET")

	// Advanced inspection endpoints
	api.HandleFunc("/consensus/logs", s.handleGetConsensusLogs).Methods("GET")
	api.HandleFunc("/node/crypto", s.handleGetNodeCrypto).Methods("GET")
	api.HandleFunc("/storage/entries", s.handleGetStorageEntries).Methods("GET")

	// Health check
	s.router.HandleFunc("/health", s.handleHealthCheck).Methods("GET")

	// WebSocket
	s.router.HandleFunc("/ws", s.handleWebSocket)

	// Static files (must be last so API routes take precedence)
	s.setupStaticRoutes()

	// Middleware
	s.router.Use(s.loggingMiddleware)
	if s.wsUpgrader.CheckOrigin != nil {
		s.router.Use(s.corsMiddleware)
	}
	if s.apiKey != "" {
		s.router.Use(s.apiKeyMiddleware)
	}
}

// Start starts the API server
func (s *Server) Start() error {
	// Start WebSocket broadcaster
	go s.broadcastLoop()

	// Start HTTP server
	go func() {
		fmt.Printf("API server listening on %s\n", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("API server error: %v\n", err)
		}
	}()

	return nil
}

// Handler functions

// ticketActionRequest is implemented by the request bodies of the ticket
// mutation endpoints (validate, consume and dispute). Each endpoint has its
// own request type, so that the wire contract of an endpoint is described by
// exactly one type.
type ticketActionRequest interface {
	// ticketID returns the ticket identifier carried by the request body.
	ticketID() string
}

// ticketID returns the ticket identifier carried by the request body.
func (r ValidateRequest) ticketID() string { return r.TicketID }

// ticketID returns the ticket identifier carried by the request body.
func (r ConsumeRequest) ticketID() string { return r.TicketID }

// ticketID returns the ticket identifier carried by the request body.
func (r DisputeRequest) ticketID() string { return r.TicketID }

// handleTicketAction implements the request handling shared by the validate,
// consume and dispute endpoints. They only differ in the request body they
// decode (the type parameter), the action they perform, the broadcast event
// they emit and the success message they return.
func handleTicketAction[T ticketActionRequest](s *Server, w http.ResponseWriter, r *http.Request, action, eventType, successMsg string, apply func(req T) error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ticketID() == "" {
		s.sendError(w, http.StatusBadRequest, "ticket_id is required")
		return
	}

	if err := apply(req); err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to %s ticket: %v", action, err))
		return
	}

	// Broadcast update
	s.broadcast <- map[string]interface{}{
		jsonKeyType:      eventType,
		jsonKeyTicketID:  req.ticketID(),
		jsonKeyTimestamp: time.Now().Unix(),
	}

	s.sendSuccess(w, successMsg, nil)
}

func (s *Server) handleValidateTicket(w http.ResponseWriter, r *http.Request) {
	handleTicketAction(s, w, r, "validate", "ticket_validated", "Ticket validated successfully",
		func(req ValidateRequest) error {
			return s.validator.ValidateTicket(req.TicketID, req.Data)
		})
}

func (s *Server) handleConsumeTicket(w http.ResponseWriter, r *http.Request) {
	handleTicketAction(s, w, r, "consume", "ticket_consumed", "Ticket consumed successfully",
		func(req ConsumeRequest) error {
			return s.validator.ConsumeTicket(req.TicketID)
		})
}

func (s *Server) handleDisputeTicket(w http.ResponseWriter, r *http.Request) {
	handleTicketAction(s, w, r, "dispute", "ticket_disputed", "Ticket disputed successfully",
		func(req DisputeRequest) error {
			return s.validator.DisputeTicket(req.TicketID)
		})
}

func (s *Server) handleSeedTickets(w http.ResponseWriter, r *http.Request) {
	seeded, err := s.validator.SeedTickets()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to seed tickets: %v", err))
		return
	}

	s.broadcast <- map[string]interface{}{
		jsonKeyType:      "tickets_seeded",
		"count":          seeded,
		jsonKeyTimestamp: time.Now().Unix(),
	}

	s.sendSuccess(w, fmt.Sprintf("Seeded %d tickets", seeded), map[string]int{"seeded": seeded})
}

func (s *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ticketID := vars["id"]

	ticket, err := s.validator.GetTicket(ticketID)
	if err != nil {
		s.sendError(w, http.StatusNotFound, fmt.Sprintf("Ticket not found: %v", err))
		return
	}

	s.sendSuccess(w, "Ticket retrieved successfully", ticket)
}

func (s *Server) handleGetAllTickets(w http.ResponseWriter, r *http.Request) {
	tickets, err := s.validator.GetAllTickets()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve tickets: %v", err))
		return
	}

	s.sendSuccess(w, "Tickets retrieved successfully", tickets)
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":         "running",
		jsonKeyTimestamp: time.Now().Unix(),
		"ws_clients":     len(s.wsClients),
	}

	s.sendSuccess(w, "Status retrieved successfully", status)
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := s.validator.GetStats()
	s.sendSuccess(w, "Stats retrieved successfully", stats)
}

func (s *Server) handleGetConsensusLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.validator.GetConsensusLogs()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve consensus logs: %v", err))
		return
	}
	s.sendSuccess(w, "Consensus logs retrieved successfully", logs)
}

func (s *Server) handleGetNodeCrypto(w http.ResponseWriter, r *http.Request) {
	info := s.validator.GetNodeCryptoInfo()
	s.sendSuccess(w, "Node crypto info retrieved successfully", info)
}

func (s *Server) handleGetStorageEntries(w http.ResponseWriter, r *http.Request) {
	entries := s.validator.GetStorageEntries()
	s.sendSuccess(w, "Storage entries retrieved successfully", entries)
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// WebSocket handler
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("WebSocket upgrade error: %v\n", err)
		return
	}

	// Register client
	s.wsMu.Lock()
	s.wsClients[conn] = true
	s.wsMu.Unlock()

	fmt.Printf("WebSocket client connected: %s\n", conn.RemoteAddr())

	// Handle client disconnect
	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, conn)
		s.wsMu.Unlock()
		if err := conn.Close(); err != nil {
			fmt.Printf("WebSocket close error: %v\n", err)
		}
		fmt.Printf("WebSocket client disconnected: %s\n", conn.RemoteAddr())
	}()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcastLoop broadcasts messages to all WebSocket clients
func (s *Server) broadcastLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.broadcast:
			// Snapshot client list under lock, then release before doing I/O.
			// This avoids holding the lock during potentially slow WriteJSON calls,
			// and eliminates the unsafe RUnlock→Lock→Unlock→RLock juggling mid-iteration.
			s.wsMu.RLock()
			clients := make([]*websocket.Conn, 0, len(s.wsClients))
			for client := range s.wsClients {
				clients = append(clients, client)
			}
			s.wsMu.RUnlock()

			// Write to each client without holding any lock
			for _, client := range clients {
				err := client.WriteJSON(msg)
				if err == nil {
					continue
				}
				fmt.Printf("WebSocket write error: %v\n", err)
				if closeErr := client.Close(); closeErr != nil {
					fmt.Printf("WebSocket close error: %v\n", closeErr)
				}
				s.wsMu.Lock()
				delete(s.wsClients, client)
				s.wsMu.Unlock()
			}
		}
	}
}

// Middleware

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("%s %s %s\n", r.Method, r.RequestURI, time.Since(start))
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// checkWSOrigin validates WebSocket connection origins to prevent cross-site
// WebSocket hijacking. It allows same-origin requests, localhost for development, and any additional origins specified
func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser clients (curl, websocat) don't send Origin
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// Same-origin check
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}

	// Allow localhost origins for development
	originHost := u.Hostname()
	if originHost == "localhost" || originHost == "127.0.0.1" || originHost == "::1" {
		return true
	}

	// Check additional allowed origins from environment
	if allowed := os.Getenv("DTVN_WS_ORIGINS"); allowed != "" {
		for _, a := range strings.Split(allowed, ",") {
			a = strings.TrimSpace(a)
			if a != "" && strings.EqualFold(origin, a) {
				return true
			}
		}
	}

	return false
}

// apiKeyMiddleware enforces API key authentication when an API key is configured.
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Exempt health check, WebSocket, and static file paths
		if path == "/health" || path == "/ws" || !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check X-API-Key header first, then Authorization: Bearer
		key := r.Header.Get("X-API-Key")
		if key == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(key), []byte(s.apiKey)) != 1 {
			s.sendError(w, http.StatusUnauthorized, "Invalid or missing API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper functions

// encodeJSON serialises payload as JSON. It is used by the response helpers so
// that an encoding failure can be reported to the client instead of producing
// a truncated response body.
func encodeJSON(payload interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeJSONResponse writes payload to w as JSON with the given status code.
// The payload is encoded before the status line is written so that an encoding
// failure is still reported to the client as a 500 instead of a broken body.
func writeJSONResponse(w http.ResponseWriter, statusCode int, payload interface{}) {
	body, err := encodeJSON(payload)
	if err != nil {
		fmt.Printf("API: failed to encode JSON response: %v\n", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		fmt.Printf("API: failed to write JSON response: %v\n", err)
	}
}

func (s *Server) sendSuccess(w http.ResponseWriter, message string, data interface{}) {
	writeJSONResponse(w, http.StatusOK, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func (s *Server) sendError(w http.ResponseWriter, statusCode int, errMsg string) {
	writeJSONResponse(w, statusCode, Response{
		Success: false,
		Error:   errMsg,
	})
}

// Broadcast sends a message to all WebSocket clients
func (s *Server) Broadcast(msg interface{}) {
	select {
	case s.broadcast <- msg:
	default:
		fmt.Println("Broadcast channel full, dropping message")
	}
}

// Close shuts down the API server
func (s *Server) Close() error {
	s.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}
