package supervisor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Server represents the supervisor HTTP server
type Server struct {
	addr          string
	router        *mux.Router
	server        *http.Server
	nodeManager   *NodeManager
	simController *SimulatorController
	wsClients     map[*websocket.Conn]bool
	wsMu          sync.RWMutex
	wsWriteMu     sync.Mutex // Serializes WebSocket writes to prevent concurrent write panic
	wsUpgrader    websocket.Upgrader
	proxyClient   *http.Client // Shared HTTP client for proxy requests (reuses connections)
	staticDir     string
	apiKey        string
	stopCh        chan struct{} // signals background goroutines to stop
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Address       string
	Port          int
	ValidatorPath string
	SimulatorPath string
	DataDir       string
	StaticDir     string
	APIKey        string // Optional API key for authentication (empty = no auth)
}

// NewServer creates a new supervisor server
func NewServer(cfg *ServerConfig) *Server {
	if cfg.Address == "" {
		cfg.Address = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	addr := fmt.Sprintf("%s:%d", cfg.Address, cfg.Port)

	s := &Server{
		addr:        addr,
		router:      mux.NewRouter(),
		wsClients:   make(map[*websocket.Conn]bool),
		proxyClient: &http.Client{Timeout: 15 * time.Second},
		staticDir:   cfg.StaticDir,
		apiKey:      cfg.APIKey,
		stopCh:      make(chan struct{}),
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     checkSupervisorWSOrigin,
		},
	}

	// Create node manager with callbacks
	s.nodeManager = NewNodeManager(&NodeManagerConfig{
		ValidatorPath: cfg.ValidatorPath,
		DataDir:       cfg.DataDir,
		APIKey:        cfg.APIKey,
		OutputCallback: func(nodeID string, line string) {
			s.broadcastWSMessage(map[string]interface{}{
				"type":    "node_output",
				"node_id": nodeID,
				"line":    line,
			})
		},
		StatusCallback: func(nodeID string, status NodeStatus) {
			// Send full node info so the frontend can add new nodes it doesn't know about
			node, err := s.nodeManager.GetNode(nodeID)
			msg := map[string]interface{}{
				"type":    "node_status",
				"node_id": nodeID,
				"status":  status,
			}
			if err == nil && node != nil {
				msg["node"] = node
			}
			s.broadcastWSMessage(msg)
		},
		ClusterCallback: func(status ClusterStatus, nodesStarted int, nodesTotal int) {
			s.broadcastWSMessage(map[string]interface{}{
				"type":          "cluster_status",
				"status":        status,
				"nodes_started": nodesStarted,
				"nodes_total":   nodesTotal,
			})
		},
	})

	// Create simulator controller with callbacks
	s.simController = NewSimulatorController(&SimulatorControllerConfig{
		SimulatorPath: cfg.SimulatorPath,
		OutputCallback: func(line string) {
			s.broadcastWSMessage(map[string]interface{}{
				"type": "simulator_output",
				"line": line,
			})
		},
		ProgressCallback: func(progress *SimulatorProgress) {
			s.broadcastWSMessage(map[string]interface{}{
				"type":     "simulator_progress",
				"progress": progress,
			})
		},
		StatusCallback: func(status SimulatorStatus) {
			s.broadcastWSMessage(map[string]interface{}{
				"type":   "simulator_status",
				"status": status,
			})
		},
		ResultCallback: func(results *SimulatorResults) {
			s.broadcastWSMessage(map[string]interface{}{
				"type":    "simulator_results",
				"results": results,
			})
		},
	})

	s.setupRoutes()

	return s
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// API routes
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Node management endpoints
	api.HandleFunc("/nodes", s.handleGetNodes).Methods("GET")
	api.HandleFunc("/nodes", s.handleStartNode).Methods("POST")
	api.HandleFunc("/nodes/{nodeID}", s.handleGetNode).Methods("GET")
	api.HandleFunc("/nodes/{nodeID}", s.handleStopNode).Methods("DELETE")
	api.HandleFunc("/nodes/{nodeID}/logs", s.handleGetNodeLogs).Methods("GET")
	api.HandleFunc("/nodes/{nodeID}/restart", s.handleRestartNode).Methods("POST")

	// Cluster management
	api.HandleFunc("/cluster/start", s.handleStartCluster).Methods("POST")
	api.HandleFunc("/cluster/stop", s.handleStopCluster).Methods("POST")

	// Simulator endpoints
	api.HandleFunc("/simulator", s.handleGetSimulatorStatus).Methods("GET")
	api.HandleFunc("/simulator/start", s.handleStartSimulator).Methods("POST")
	api.HandleFunc("/simulator/stop", s.handleStopSimulator).Methods("POST")
	api.HandleFunc("/simulator/output", s.handleGetSimulatorOutput).Methods("GET")
	api.HandleFunc("/simulator/results", s.handleGetSimulatorResults).Methods("GET")

	// Ticket proxy endpoints (forward to running validator nodes)
	api.HandleFunc("/tickets", s.handleProxyGetAllTickets).Methods("GET")
	api.HandleFunc("/tickets/validate", s.handleProxyValidateTicket).Methods("POST")
	api.HandleFunc("/tickets/validate-via-node", s.handleProxyValidateTicketViaNode).Methods("POST")
	api.HandleFunc("/tickets/consume", s.handleProxyConsumeTicket).Methods("POST")
	api.HandleFunc("/tickets/dispute", s.handleProxyDisputeTicket).Methods("POST")
	api.HandleFunc("/tickets/seed", s.handleProxySeedTickets).Methods("POST")
	api.HandleFunc("/tickets/{id}", s.handleProxyGetTicket).Methods("GET")

	// Stats proxy endpoint
	api.HandleFunc("/stats", s.handleProxyGetStats).Methods("GET")

	// Peers proxy endpoint
	api.HandleFunc("/peers", s.handleProxyGetPeers).Methods("GET")

	// Config proxy endpoint
	api.HandleFunc("/config", s.handleProxyGetConfig).Methods("GET")

	// Advanced inspection proxy endpoints
	api.HandleFunc("/consensus/logs", s.handleProxyGetConsensusLogs).Methods("GET")
	api.HandleFunc("/node/crypto", s.handleProxyGetNodeCrypto).Methods("GET")
	api.HandleFunc("/storage/entries", s.handleProxyGetStorageEntries).Methods("GET")

	// Supervisor status
	api.HandleFunc("/status", s.handleGetStatus).Methods("GET")

	// WebSocket endpoint
	s.router.HandleFunc("/ws", s.handleWebSocket)

	// Middleware
	if s.apiKey != "" {
		s.router.Use(s.apiKeyMiddleware)
	}

	// Static file serving (must be last)
	s.setupStaticRoutes()
}

// setupStaticRoutes configures static file serving
func (s *Server) setupStaticRoutes() {
	staticDir := s.staticDir
	if staticDir == "" {
		staticDir = "web/static"
	}

	// Check if the directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try alternative paths
		alternatives := []string{
			"../../web/static",
			"../web/static",
			"./web/static",
		}

		for _, alt := range alternatives {
			if absPath, err := filepath.Abs(alt); err == nil {
				if _, err := os.Stat(absPath); err == nil {
					staticDir = absPath
					break
				}
			}
		}
	}

	if _, err := os.Stat(staticDir); err == nil {
		fileServer := http.FileServer(http.Dir(staticDir))
		s.router.PathPrefix("/").Handler(fileServer)
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// Start background monitor that detects PBFT primary changes via view change
	go s.monitorPrimaryStatus()

	fmt.Printf("Supervisor server starting on http://%s\n", s.addr)
	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	// Stop background goroutines
	close(s.stopCh)

	// Stop all managed nodes
	s.nodeManager.StopAllNodes()

	// Stop simulator if running
	if s.simController.GetStatus() == SimulatorStatusRunning {
		s.simController.Stop()
	}

	// Close all WebSocket connections
	s.wsMu.Lock()
	for conn := range s.wsClients {
		conn.Close()
	}
	s.wsClients = make(map[*websocket.Conn]bool)
	s.wsMu.Unlock()

	// Shutdown HTTP server
	return s.server.Shutdown(ctx)
}

// monitorPrimaryStatus periodically polls running validator nodes to detect
// PBFT primary changes (e.g., after a view change when the old primary went down).
// When a change is detected, it updates the manager and broadcasts a WebSocket event
// so the dashboard and network graph reflect the new primary in real time.
func (s *Server) monitorPrimaryStatus() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.refreshPrimaryStatus()
		}
	}
}

// refreshPrimaryStatus queries each running node's stats endpoint and updates
// the supervisor's primary tracking if the consensus layer has changed the primary.
func (s *Server) refreshPrimaryStatus() {
	nodes := s.nodeManager.GetAllNodes()

	for _, node := range nodes {
		if node.Status != NodeStatusRunning {
			continue
		}

		url := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)
		isPrimary, err := s.checkNodeIsPrimary(url)
		if err != nil {
			continue
		}

		if s.nodeManager.UpdatePrimaryStatus(node.ID, isPrimary) {
			// Primary status changed — broadcast to all connected dashboards
			updatedNode, err := s.nodeManager.GetNode(node.ID)
			if err != nil {
				continue
			}
			s.broadcastWSMessage(map[string]interface{}{
				"type":    "node_status",
				"node_id": node.ID,
				"status":  updatedNode.Status,
				"node":    updatedNode,
			})
		}
	}
}

// API Handlers

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodeManager.GetAllNodes()
	simStatus := s.simController.GetStatus()

	status := map[string]interface{}{
		"supervisor":       "running",
		"nodes_count":      len(nodes),
		"nodes":            nodes,
		"simulator_status": simStatus,
	}

	s.writeJSON(w, status)
}

func (s *Server) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	// Refresh primary status from live consensus state before responding,
	// so the frontend sees correct primary/replica roles after view changes.
	s.refreshPrimaryStatus()
	nodes := s.nodeManager.GetAllNodes()
	s.writeJSON(w, nodes)
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeID"]

	node, err := s.nodeManager.GetNode(nodeID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, node)
}

func (s *Server) handleStartNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID        string `json:"node_id"`
		IsPrimary     bool   `json:"is_primary"`
		IsBootstrap   bool   `json:"is_bootstrap"`
		BootstrapAddr string `json:"bootstrap_addr"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NodeID == "" {
		s.writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}

	if err := s.nodeManager.StartNode(req.NodeID, req.IsPrimary, req.IsBootstrap, req.BootstrapAddr); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]string{
		"status":  "starting",
		"node_id": req.NodeID,
	})
}

func (s *Server) handleStopNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeID"]

	if err := s.nodeManager.StopNode(nodeID); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]string{
		"status":  "stopping",
		"node_id": nodeID,
	})
}

func (s *Server) handleRestartNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeID"]

	// Get existing node info
	node, err := s.nodeManager.GetNode(nodeID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Stop if running
	if node.Status == NodeStatusRunning {
		if err := s.nodeManager.StopNode(nodeID); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Wait for stop
		time.Sleep(1 * time.Second)
	}

	// Start node
	if err := s.nodeManager.StartNode(nodeID, node.IsPrimary, node.IsBootstrap, ""); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]string{
		"status":  "restarting",
		"node_id": nodeID,
	})
}

func (s *Server) handleGetNodeLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeID := vars["nodeID"]

	logs, err := s.nodeManager.GetNodeLogs(nodeID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, map[string]interface{}{
		"node_id": nodeID,
		"logs":    logs,
	})
}

func (s *Server) handleStartCluster(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeCount int `json:"node_count"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NodeCount < 1 {
		req.NodeCount = 4 // Default to 4 nodes
	}

	// StartCluster is now asynchronous - it returns immediately and starts
	// nodes in the background. Progress is sent via WebSocket cluster_status events.
	if err := s.nodeManager.StartCluster(req.NodeCount); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]interface{}{
		"status":     "starting",
		"node_count": req.NodeCount,
	})
}

func (s *Server) handleStopCluster(w http.ResponseWriter, r *http.Request) {
	s.nodeManager.StopAllNodes()

	s.writeJSON(w, map[string]string{
		"status": "stopping",
	})
}

// Simulator handlers

func (s *Server) handleGetSimulatorStatus(w http.ResponseWriter, r *http.Request) {
	state := s.simController.GetFullState()
	s.writeJSON(w, state)
}

func (s *Server) handleStartSimulator(w http.ResponseWriter, r *http.Request) {
	var config SimulatorConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		// Use default config if no body provided
		config = *DefaultSimulatorConfig()
	}

	// Validate configuration
	if err := ValidateConfig(&config); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.simController.Start(&config); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]interface{}{
		"status": "started",
		"config": config,
	})
}

func (s *Server) handleStopSimulator(w http.ResponseWriter, r *http.Request) {
	if err := s.simController.Stop(); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]string{
		"status": "stopping",
	})
}

func (s *Server) handleGetSimulatorOutput(w http.ResponseWriter, r *http.Request) {
	output := s.simController.GetOutput()
	s.writeJSON(w, map[string]interface{}{
		"output": output,
	})
}

func (s *Server) handleGetSimulatorResults(w http.ResponseWriter, r *http.Request) {
	results := s.simController.GetResults()
	if results == nil {
		s.writeError(w, http.StatusNotFound, "no results available")
		return
	}
	s.writeJSON(w, results)
}

// WebSocket handling

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.wsMu.Lock()
	s.wsClients[conn] = true
	s.wsMu.Unlock()

	// Send initial state
	s.sendWSMessage(conn, map[string]interface{}{
		"type":             "connected",
		"nodes":            s.nodeManager.GetAllNodes(),
		"simulator_status": s.simController.GetStatus(),
		"cluster_status":   s.nodeManager.GetClusterStatus(),
	})

	// Handle incoming messages
	go s.handleWSMessages(conn)
}

func (s *Server) handleWSMessages(conn *websocket.Conn) {
	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, conn)
		s.wsMu.Unlock()
		conn.Close()
	}()

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("WebSocket error: %v\n", err)
			}
			break
		}

		// Handle ping messages
		if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
			s.sendWSMessage(conn, map[string]interface{}{
				"type": "pong",
			})
		}
	}
}

func (s *Server) sendWSMessage(conn *websocket.Conn, msg map[string]interface{}) {
	s.wsWriteMu.Lock()
	defer s.wsWriteMu.Unlock()
	conn.WriteJSON(msg)
}

func (s *Server) broadcastWSMessage(msg map[string]interface{}) {
	s.wsMu.RLock()
	clients := make([]*websocket.Conn, 0, len(s.wsClients))
	for conn := range s.wsClients {
		clients = append(clients, conn)
	}
	s.wsMu.RUnlock()

	// Write to all clients with write lock to prevent concurrent writes
	s.wsWriteMu.Lock()
	defer s.wsWriteMu.Unlock()
	for _, conn := range clients {
		conn.WriteJSON(msg)
	}
}

// checkSupervisorWSOrigin validates WebSocket connection origins to prevent cross-site
// WebSocket hijacking. Allows same-origin, localhost, and origins listed in
// the DTVN_WS_ORIGINS environment variable (comma-separated).
// Requests without an Origin header (non-browser clients) are always allowed.
func checkSupervisorWSOrigin(r *http.Request) bool {
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
// Exempt paths: /ws (WebSocket) and static files (non-/api/ paths).
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Exempt WebSocket and static file paths
		if path == "/ws" || !strings.HasPrefix(path, "/api/") {
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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid or missing API key",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper methods

func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
