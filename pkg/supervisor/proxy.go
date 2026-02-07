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

// getRunningNodeURL returns the API URL of the first running node
func (s *Server) getRunningNodeURL() (string, error) {
	nodes := s.nodeManager.GetAllNodes()

	for _, node := range nodes {
		if node.Status == NodeStatusRunning {
			return fmt.Sprintf("http://127.0.0.1:%d", node.APIPort), nil
		}
	}

	return "", fmt.Errorf("no running nodes available")
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

// proxyTicketRequest proxies a ticket mutation request and broadcasts a WebSocket event on success
func (s *Server) proxyTicketRequest(w http.ResponseWriter, r *http.Request, endpoint string, eventType string) {
	nodeURL, err := s.getRunningNodeURL()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "No running nodes available. Start a cluster first.")
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
