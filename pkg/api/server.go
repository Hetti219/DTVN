package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Server represents the API server
type Server struct {
	router      *mux.Router
	httpServer  *http.Server
	validator   ValidatorInterface
	wsClients   map[*websocket.Conn]bool
	wsUpgrader  websocket.Upgrader
	wsMu        sync.RWMutex
	broadcast   chan interface{}
	ctx         context.Context
	cancel      context.CancelFunc
}

// ValidatorInterface defines the interface for validator operations
type ValidatorInterface interface {
	ValidateTicket(ticketID string, data []byte) error
	ConsumeTicket(ticketID string) error
	DisputeTicket(ticketID string) error
	GetTicket(ticketID string) (interface{}, error)
	GetAllTickets() ([]interface{}, error)
	GetStats() map[string]interface{}
}

// Config holds API server configuration
type Config struct {
	Address   string
	Port      int
	EnableCORS bool
}

// Request/Response structures

type ValidateRequest struct {
	TicketID string `json:"ticket_id"`
	Data     []byte `json:"data"`
}

type ConsumeRequest struct {
	TicketID string `json:"ticket_id"`
}

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
			CheckOrigin: func(r *http.Request) bool {
				return cfg.EnableCORS // Allow all origins if CORS enabled
			},
		},
		broadcast: make(chan interface{}, 100),
		ctx:       serverCtx,
		cancel:    cancel,
	}

	// Setup routes
	s.setupRoutes()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Address, cfg.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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
	api.HandleFunc("/tickets/{id}", s.handleGetTicket).Methods("GET")
	api.HandleFunc("/tickets", s.handleGetAllTickets).Methods("GET")

	// Node status
	api.HandleFunc("/status", s.handleGetStatus).Methods("GET")
	api.HandleFunc("/stats", s.handleGetStats).Methods("GET")

	// Health check
	s.router.HandleFunc("/health", s.handleHealthCheck).Methods("GET")

	// WebSocket
	s.router.HandleFunc("/ws", s.handleWebSocket)

	// Middleware
	s.router.Use(s.loggingMiddleware)
	if s.wsUpgrader.CheckOrigin != nil {
		s.router.Use(s.corsMiddleware)
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

func (s *Server) handleValidateTicket(w http.ResponseWriter, r *http.Request) {
	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TicketID == "" {
		s.sendError(w, http.StatusBadRequest, "ticket_id is required")
		return
	}

	if err := s.validator.ValidateTicket(req.TicketID, req.Data); err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to validate ticket: %v", err))
		return
	}

	// Broadcast update
	s.broadcast <- map[string]interface{}{
		"type":      "ticket_validated",
		"ticket_id": req.TicketID,
		"timestamp": time.Now().Unix(),
	}

	s.sendSuccess(w, "Ticket validated successfully", nil)
}

func (s *Server) handleConsumeTicket(w http.ResponseWriter, r *http.Request) {
	var req ConsumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TicketID == "" {
		s.sendError(w, http.StatusBadRequest, "ticket_id is required")
		return
	}

	if err := s.validator.ConsumeTicket(req.TicketID); err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to consume ticket: %v", err))
		return
	}

	// Broadcast update
	s.broadcast <- map[string]interface{}{
		"type":      "ticket_consumed",
		"ticket_id": req.TicketID,
		"timestamp": time.Now().Unix(),
	}

	s.sendSuccess(w, "Ticket consumed successfully", nil)
}

func (s *Server) handleDisputeTicket(w http.ResponseWriter, r *http.Request) {
	var req DisputeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TicketID == "" {
		s.sendError(w, http.StatusBadRequest, "ticket_id is required")
		return
	}

	if err := s.validator.DisputeTicket(req.TicketID); err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to dispute ticket: %v", err))
		return
	}

	// Broadcast update
	s.broadcast <- map[string]interface{}{
		"type":      "ticket_disputed",
		"ticket_id": req.TicketID,
		"timestamp": time.Now().Unix(),
	}

	s.sendSuccess(w, "Ticket disputed successfully", nil)
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
		"status":    "running",
		"timestamp": time.Now().Unix(),
		"ws_clients": len(s.wsClients),
	}

	s.sendSuccess(w, "Status retrieved successfully", status)
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := s.validator.GetStats()
	s.sendSuccess(w, "Stats retrieved successfully", stats)
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
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
		conn.Close()
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
			s.wsMu.RLock()
			for client := range s.wsClients {
				err := client.WriteJSON(msg)
				if err != nil {
					fmt.Printf("WebSocket write error: %v\n", err)
					client.Close()
					s.wsMu.RUnlock()
					s.wsMu.Lock()
					delete(s.wsClients, client)
					s.wsMu.Unlock()
					s.wsMu.RLock()
				}
			}
			s.wsMu.RUnlock()
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper functions

func (s *Server) sendSuccess(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func (s *Server) sendError(w http.ResponseWriter, statusCode int, error string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Error:   error,
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
