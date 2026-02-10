package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	hasPrimary := false
	for _, node := range nodes {
		if node.IsPrimary {
			hasPrimary = true
			if node.Status != NodeStatusRunning {
				return "", fmt.Errorf("primary node is not running (status: %s). Start a cluster first", node.Status)
			}
			url := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)
			if !s.isNodeReachable(url) {
				return "", fmt.Errorf("primary node is still starting up, please try again in a few seconds")
			}
			return url, nil
		}
	}

	if !hasPrimary {
		return "", fmt.Errorf("no primary node found. Start a cluster first")
	}
	return "", fmt.Errorf("no running nodes available")
}

// isNodeReachable checks if a node's API server is actually listening
func (s *Server) isNodeReachable(baseURL string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// proxyRequest forwards a request to a running validator node
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, endpoint string) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "No running nodes available. Start a cluster first.")
		return
	}

	targetURL := fmt.Sprintf("%s/api/v1%s", nodeURL, endpoint)

	// Create new request
	var body io.Reader
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to read request body")
			return
		}
		body = bytes.NewReader(bodyBytes)
	}

	proxyReq, err := http.NewRequest(r.Method, targetURL, body)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}

	// Copy headers
	proxyReq.Header = r.Header.Clone()

	// Make request
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
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
	io.Copy(w, resp.Body)
}

// proxyTicketRequest proxies a ticket mutation request to the primary node
// and broadcasts a WebSocket event on success
func (s *Server) proxyTicketRequest(w http.ResponseWriter, r *http.Request, endpoint string, eventType string) {
	nodeURL, err := s.getPrimaryNodeURL()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Read body to extract ticket_id for the WebSocket event
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var reqBody struct {
		TicketID string `json:"ticket_id"`
	}
	json.Unmarshal(bodyBytes, &reqBody)

	// Proxy the request
	targetURL := fmt.Sprintf("%s/api/v1%s", nodeURL, endpoint)
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}
	proxyReq.Header = r.Header.Clone()

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
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
	io.Copy(w, resp.Body)

	// Broadcast WebSocket event on success
	if resp.StatusCode == http.StatusOK {
		s.broadcastWSMessage(map[string]interface{}{
			"type":      eventType,
			"ticket_id": reqBody.TicketID,
			"timestamp": time.Now().Unix(),
		})
	}
}

// handleProxySeedTickets seeds tickets on ALL running nodes (not just primary).
// Seeding is a local operation — each node loads the same deterministic data independently.
func (s *Server) handleProxySeedTickets(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodeManager.GetAllNodes()

	var seededTotal int
	var nodeResults []map[string]interface{}
	var lastErr error

	for _, node := range nodes {
		if node.Status != NodeStatusRunning {
			continue
		}

		nodeBaseURL := fmt.Sprintf("http://127.0.0.1:%d", node.APIPort)
		if !s.isNodeReachable(nodeBaseURL) {
			continue
		}

		seedURL := fmt.Sprintf("%s/api/v1/tickets/seed", nodeBaseURL)
		req, err := http.NewRequest("POST", seedURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		seeded := 0
		if data, ok := result["data"].(map[string]interface{}); ok {
			if val, ok := data["seeded"].(float64); ok {
				seeded = int(val)
			}
		}
		seededTotal += seeded
		nodeResults = append(nodeResults, map[string]interface{}{
			"node_id": node.ID,
			"seeded":  seeded,
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
		"type":      "tickets_seeded",
		"count":     seededTotal,
		"nodes":     len(nodeResults),
		"timestamp": time.Now().Unix(),
	})

	s.writeJSON(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Seeded tickets on %d nodes", len(nodeResults)),
		"data": map[string]interface{}{
			"total_seeded": seededTotal,
			"nodes":        nodeResults,
		},
	})
}

// Ticket proxy handlers

func (s *Server) handleProxyGetAllTickets(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/tickets")
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
	s.proxyRequest(w, r, fmt.Sprintf("/tickets/%s", ticketID))
}

// Stats/Peers/Config proxy handlers

func (s *Server) handleProxyGetStats(w http.ResponseWriter, r *http.Request) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		// Return supervisor-level stats if no nodes running
		s.writeJSON(w, map[string]interface{}{
			"node_id":      "supervisor",
			"peer_count":   0,
			"is_primary":   false,
			"current_view": 0,
			"sequence":     0,
			"mode":         "supervisor",
			"nodes_count":  len(s.nodeManager.GetAllNodes()),
		})
		return
	}

	// Proxy to running node
	targetURL := fmt.Sprintf("%s/api/v1/stats", nodeURL)
	resp, err := http.Get(targetURL)
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
		dataField["mode"] = "supervisor"
		s.writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "Stats retrieved successfully",
			"data":    dataField,
		})
	} else {
		data["mode"] = "supervisor"
		s.writeJSON(w, data)
	}
}

func (s *Server) handleProxyGetPeers(w http.ResponseWriter, r *http.Request) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		// Return empty peers list if no nodes running
		s.writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "No running nodes",
			"data":    []interface{}{},
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
			"success": true,
			"message": "Supervisor config",
			"data": map[string]interface{}{
				"mode":        "supervisor",
				"nodes_count": len(s.nodeManager.GetAllNodes()),
			},
		})
		return
	}

	s.proxyRequest(w, r, "/config")
	_ = nodeURL // used in proxyRequest
}
