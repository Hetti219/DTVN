package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// getRunningNodeURL returns the API URL of a running node, preferring the primary.
// It verifies the node's API is actually reachable before returning, since a node's
// status is set to "running" as soon as the OS process starts, before the API server
// is listening.
func (s *Server) getRunningNodeURL() (string, error) {
	nodes := s.nodeManager.GetAllNodes()

	var primaryURL string
	var fallbackURL string
	hasRunningNodes := false

	for _, node := range nodes {
		if node.Status == NodeStatusRunning {
			hasRunningNodes = true
			url := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)

			if !s.isNodeReachable(url) {
				continue
			}

			if node.IsPrimary {
				primaryURL = url
			} else if fallbackURL == "" {
				fallbackURL = url
			}
		}
	}

	if primaryURL != "" {
		return primaryURL, nil
	}
	if fallbackURL != "" {
		return fallbackURL, nil
	}

	if hasRunningNodes {
		return "", fmt.Errorf("nodes are still starting up, please try again in a few seconds")
	}
	return "", fmt.Errorf("no running nodes available")
}

// getPrimaryNodeURL returns the API URL of the primary node only.
// Ticket mutations must go through the primary for PBFT consensus.
// Falling back to a non-primary would just cause a 5-second forwarding timeout.
func (s *Server) getPrimaryNodeURL() (string, error) {
	nodes := s.nodeManager.GetAllNodes()

	// Phase 1: Dynamic discovery — query running nodes to find who is
	// actually the PBFT primary. This reflects view changes at the consensus
	// layer (e.g., after the original primary went down and a new one was elected).
	for _, node := range nodes {
		if node.Status != NodeStatusRunning {
			continue
		}
		url := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)
		if isPrimary, err := s.checkNodeIsPrimary(url); err == nil && isPrimary {
			return url, nil
		}
	}

	// Phase 2: Fall back to static IsPrimary flag from cluster startup.
	// This handles the case where nodes haven't fully started their API servers
	// yet but were configured as primary during cluster creation.
	var primary *ManagedNode
	for _, node := range nodes {
		if !node.IsPrimary {
			continue
		}
		primary = node
		break
	}
	if primary == nil {
		return "", fmt.Errorf("no primary node found. Start a cluster first")
	}
	if primary.Status != NodeStatusRunning {
		return "", fmt.Errorf("primary node is not running (status: %s). Start a cluster first", primary.Status)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", primary.APIPort)
	if !s.isNodeReachable(url) {
		return "", fmt.Errorf("primary node is still starting up, please try again in a few seconds")
	}
	return url, nil
}

// isNodeReachable checks if a node's API server is actually listening.
// Uses context timeout with the shared client to avoid creating new clients.
func (s *Server) isNodeReachable(baseURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/health", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := s.proxyClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// checkNodeIsPrimary queries a node's stats endpoint to determine if it
// considers itself the PBFT primary. This reflects live consensus state
// including view changes that happen after cluster startup.
func (s *Server) checkNodeIsPrimary(baseURL string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v1/stats", http.NoBody)
	if err != nil {
		return false, err
	}
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	resp, err := s.proxyClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("stats returned %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			IsPrimary bool `json:"is_primary"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Data.IsPrimary, nil
}

// proxyRequest forwards a request to a running validator node
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, endpoint string) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "No running nodes available. Start a cluster first.")
		return
	}

	targetURL := fmt.Sprintf("%s/api/v1%s", nodeURL, endpoint)

	// Read body with size limit (1MB)
	var body io.Reader
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to read request body")
			return
		}
		body = bytes.NewReader(bodyBytes)
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, body) // #nosec G107 -- targetURL is constructed from internal 127.0.0.1 node addresses, not user input
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}

	// Copy headers
	proxyReq.Header = r.Header.Clone()

	// Make request using shared client
	resp, err := s.proxyClient.Do(proxyReq) // #nosec G704 -- internal addresses
	if err != nil {
		s.writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to connect to validator node: %v", err))
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code and body
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Printf("Supervisor: failed to copy proxied response body: %v\n", err)
	}
}

// proxyRequestToPrimary forwards a read request to the primary node for consistency.
// Falls back to any running node if the primary is unavailable.
func (s *Server) proxyRequestToPrimary(w http.ResponseWriter, r *http.Request, endpoint string) {
	nodeURL, err := s.getPrimaryNodeURL()
	if err != nil {
		// Fallback to any running node rather than failing entirely
		nodeURL, err = s.getRunningNodeURL()
		if err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "No running nodes available. Start a cluster first.")
			return
		}
	}

	targetURL := fmt.Sprintf("%s/api/v1%s", nodeURL, endpoint)

	// Read body with size limit (1MB)
	var body io.Reader
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to read request body")
			return
		}
		body = bytes.NewReader(bodyBytes)
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, body) // #nosec G107 G704 -- targetURL is constructed from internal 127.0.0.1 node addresses, not user input
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}

	proxyReq.Header = r.Header.Clone()

	resp, err := s.proxyClient.Do(proxyReq) // #nosec G704 -- internal addresses
	if err != nil {
		s.writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to connect to validator node: %v", err))
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Printf("Supervisor: failed to copy proxied response body: %v\n", err)
	}
}

// proxyTicketRequest proxies a ticket mutation request to the primary node
// and broadcasts a WebSocket event on success
func (s *Server) proxyTicketRequest(w http.ResponseWriter, r *http.Request, endpoint, eventType string) {
	nodeURL, err := s.getPrimaryNodeURL()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Read body with size limit to extract ticket_id for the WebSocket event
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var reqBody struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		// The upstream node validates the payload and reports malformed requests;
		// a parse failure here only means the ticket_id for the WebSocket event is
		// unknown, so the request is still forwarded.
		fmt.Printf("Supervisor: failed to parse ticket_id from request body: %v\n", err)
	}

	// Proxy the request using shared client (reuses TCP connections)
	targetURL := fmt.Sprintf("%s/api/v1%s", nodeURL, endpoint)
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(bodyBytes)) // #nosec G107 G704 -- targetURL is constructed from internal 127.0.0.1 node addresses, not user input
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}
	proxyReq.Header = r.Header.Clone()

	resp, err := s.proxyClient.Do(proxyReq) // #nosec G704 -- internal addresses
	if err != nil {
		s.writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to connect to validator node: %v", err))
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code and body
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Printf("Supervisor: failed to copy proxied response body: %v\n", err)
	}

	// Broadcast WebSocket event on success
	if resp.StatusCode == http.StatusOK {
		s.broadcastWSMessage(map[string]interface{}{
			jsonKeyType:      eventType,
			jsonKeyTicketID:  reqBody.TicketID,
			jsonKeyTimestamp: time.Now().Unix(),
		})
	}
}

// seedResult holds the result of seeding a single node.
type seedResult struct {
	NodeID string
	Seeded int
	Err    error
}

// handleProxySeedTickets seeds tickets on ALL running nodes (not just primary).
// Seeding is a local operation — each node loads the same deterministic data
// independently, so requests are sent in parallel for faster completion.
func (s *Server) handleProxySeedTickets(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodeManager.GetAllNodes()

	// Collect reachable nodes first
	type targetNode struct {
		id      string
		seedURL string
	}
	var targets []targetNode
	for _, node := range nodes {
		if node.Status != NodeStatusRunning {
			continue
		}
		nodeBaseURL := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)
		if !s.isNodeReachable(nodeBaseURL) {
			continue
		}
		targets = append(targets, targetNode{
			id:      node.ID,
			seedURL: fmt.Sprintf("%s/api/v1/tickets/seed", nodeBaseURL),
		})
	}

	if len(targets) == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No running nodes available. Start a cluster first.")
		return
	}

	// Seed all nodes in parallel with concurrency limit of 10
	results := make([]seedResult, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, t targetNode) {
			defer wg.Done()
			sem <- struct{}{}
			results[idx] = s.seedNode(t.id, t.seedURL)
			<-sem
		}(i, target)
	}
	wg.Wait()

	// Aggregate results
	var seededTotal int
	var nodeResults []map[string]interface{}
	var lastErr error

	for _, res := range results {
		if res.Err != nil {
			lastErr = res.Err
			continue
		}
		seededTotal += res.Seeded
		nodeResults = append(nodeResults, map[string]interface{}{
			jsonKeyNodeID: res.NodeID,
			"seeded":      res.Seeded,
		})
	}

	if len(nodeResults) == 0 {
		errMsg := "No running nodes available. Start a cluster first."
		if lastErr != nil {
			errMsg = fmt.Sprintf("Failed to seed: %v", lastErr)
		}
		s.writeError(w, http.StatusServiceUnavailable, errMsg)
		return
	}

	s.broadcastWSMessage(map[string]interface{}{
		jsonKeyType:      "tickets_seeded",
		"count":          seededTotal,
		jsonKeyNodes:     len(nodeResults),
		jsonKeyTimestamp: time.Now().Unix(),
	})

	s.writeJSON(w, map[string]interface{}{
		jsonKeySuccess: true,
		jsonKeyMessage: fmt.Sprintf("Seeded tickets on %d nodes", len(nodeResults)),
		jsonKeyData: map[string]interface{}{
			"total_seeded": seededTotal,
			jsonKeyNodes:   nodeResults,
		},
	})
}

// seedNode sends a seed request to a single node and returns the result.
// Uses a per-request context timeout with the shared client for connection reuse.
func (s *Server) seedNode(nodeID, seedURL string) seedResult {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", seedURL, http.NoBody) // #nosec G107 G704 -- seedURL is constructed from internal 127.0.0.1 node addresses, not user input
	if err != nil {
		return seedResult{NodeID: nodeID, Err: err}
	}
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}

	resp, err := s.proxyClient.Do(req)
	if err != nil {
		return seedResult{NodeID: nodeID, Err: err}
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return seedResult{NodeID: nodeID, Err: fmt.Errorf("failed to parse seed response: %w", err)}
	}

	seeded := 0
	if data, ok := result["data"].(map[string]interface{}); ok {
		if val, ok := data["seeded"].(float64); ok {
			seeded = int(val)
		}
	}
	return seedResult{NodeID: nodeID, Seeded: seeded}
}

// newValidateProxyRequest builds the POST request that forwards a ticket
// validation to a specific node.
func newValidateProxyRequest(nodeURL, ticketID string) (*http.Request, error) {
	body, err := json.Marshal(map[string]string{jsonKeyTicketID: ticketID})
	if err != nil {
		return nil, fmt.Errorf("failed to encode validation request: %w", err)
	}

	targetURL := fmt.Sprintf("%s/api/v1/tickets/validate", nodeURL)
	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(body)) // #nosec G107 G704 -- targetURL is constructed from internal 127.0.0.1 node addresses, not user input
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// handleProxyValidateTicketViaNode routes a validation request to a specific node
// instead of always going to the primary. This allows testing scenarios like
// double validation, non-primary validation (gossip forwarding), etc.
func (s *Server) handleProxyValidateTicketViaNode(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var reqBody struct {
		TicketID string `json:"ticket_id"`
		NodeID   string `json:"node_id"`
	}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if reqBody.TicketID == "" {
		s.writeError(w, http.StatusBadRequest, "ticket_id is required")
		return
	}
	if reqBody.NodeID == "" {
		s.writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}

	// Look up the target node
	node, err := s.nodeManager.GetNode(reqBody.NodeID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("Node not found: %s", reqBody.NodeID))
		return
	}
	if node.Status != NodeStatusRunning {
		s.writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("Node %s is not running (status: %s)", reqBody.NodeID, node.Status))
		return
	}

	nodeURL := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)
	if !s.isNodeReachable(nodeURL) {
		s.writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("Node %s is not reachable yet", reqBody.NodeID))
		return
	}

	// Proxy the validate request to the specific node
	proxyReq, err := newValidateProxyRequest(nodeURL, reqBody.TicketID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.apiKey != "" {
		proxyReq.Header.Set("X-API-Key", s.apiKey)
	}

	resp, err := s.proxyClient.Do(proxyReq)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to connect to node %s: %v", reqBody.NodeID, err))
		return
	}
	defer resp.Body.Close()

	// Copy response
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Printf("Supervisor: failed to copy proxied response body: %v\n", err)
	}

	// Broadcast WebSocket event on success
	if resp.StatusCode == http.StatusOK {
		s.broadcastWSMessage(map[string]interface{}{
			jsonKeyType:      "ticket_validated",
			jsonKeyTicketID:  reqBody.TicketID,
			jsonKeyNodeID:    reqBody.NodeID,
			jsonKeyTimestamp: time.Now().Unix(),
		})
	}
}

// Ticket proxy handlers

func (s *Server) handleProxyGetAllTickets(w http.ResponseWriter, r *http.Request) {
	// Read from primary for consistency — the primary always has the latest
	// committed state, while secondaries may lag behind state replication.
	s.proxyRequestToPrimary(w, r, "/tickets")
}

func (s *Server) handleProxyValidateTicket(w http.ResponseWriter, r *http.Request) {
	s.proxyTicketRequest(w, r, "/tickets/validate", "ticket_validated")
}

func (s *Server) handleProxyConsumeTicket(w http.ResponseWriter, r *http.Request) {
	s.proxyTicketRequest(w, r, "/tickets/consume", "ticket_consumed")
}

func (s *Server) handleProxyDisputeTicket(w http.ResponseWriter, r *http.Request) {
	s.proxyTicketRequest(w, r, "/tickets/dispute", "ticket_disputed")
}

func (s *Server) handleProxyGetTicket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ticketID := vars["id"]
	s.proxyRequestToPrimary(w, r, fmt.Sprintf("/tickets/%s", ticketID))
}

// Stats/Peers/Config proxy handlers

func (s *Server) handleProxyGetStats(w http.ResponseWriter, r *http.Request) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		// Return supervisor-level stats if no nodes running
		s.writeJSON(w, map[string]interface{}{
			jsonKeyNodeID:     supervisorIdentifier,
			"peer_count":      0,
			"is_primary":      false,
			"current_view":    0,
			"sequence":        0,
			"mode":            supervisorIdentifier,
			jsonKeyNodesCount: len(s.nodeManager.GetAllNodes()),
		})
		return
	}

	// Proxy to running node
	targetURL := fmt.Sprintf("%s/api/v1/stats", nodeURL)
	statsReq, err := http.NewRequest("GET", targetURL, http.NoBody) // #nosec G107 G704 -- targetURL is constructed from internal 127.0.0.1 node addresses, not user input
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "Failed to get stats from node")
		return
	}
	if s.apiKey != "" {
		statsReq.Header.Set("X-API-Key", s.apiKey)
	}
	resp, err := s.proxyClient.Do(statsReq)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "Failed to get stats from node")
		return
	}
	defer resp.Body.Close()

	// Parse and forward
	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to parse stats")
		return
	}

	// Add supervisor mode indicator
	if dataField, ok := data["data"].(map[string]interface{}); ok {
		dataField["mode"] = supervisorIdentifier
		s.writeJSON(w, map[string]interface{}{
			jsonKeySuccess: true,
			jsonKeyMessage: "Stats retrieved successfully",
			jsonKeyData:    dataField,
		})
	} else {
		data["mode"] = supervisorIdentifier
		s.writeJSON(w, data)
	}
}

func (s *Server) handleProxyGetPeers(w http.ResponseWriter, r *http.Request) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		// Return empty peers list if no nodes running
		s.writeJSON(w, map[string]interface{}{
			jsonKeySuccess: true,
			jsonKeyMessage: "No running nodes",
			jsonKeyData:    []interface{}{},
		})
		return
	}

	s.proxyRequest(w, r, "/peers")
	_ = nodeURL // used in proxyRequest
}

func (s *Server) handleProxyGetConfig(w http.ResponseWriter, r *http.Request) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		// Return supervisor config if no nodes running
		s.writeJSON(w, map[string]interface{}{
			jsonKeySuccess: true,
			jsonKeyMessage: "Supervisor config",
			jsonKeyData: map[string]interface{}{
				"mode":            supervisorIdentifier,
				jsonKeyNodesCount: len(s.nodeManager.GetAllNodes()),
			},
		})
		return
	}

	s.proxyRequest(w, r, "/config")
	_ = nodeURL // used in proxyRequest
}

// Advanced inspection proxy handlers

func (s *Server) handleProxyGetConsensusLogs(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/consensus/logs")
}

func (s *Server) handleProxyGetNodeCrypto(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/node/crypto")
}

func (s *Server) handleProxyGetStorageEntries(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/storage/entries")
}
