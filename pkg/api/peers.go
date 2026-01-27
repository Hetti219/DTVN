package api

import (
	"encoding/json"
	"net/http"
)

// PeerInfo represents information about a connected peer
type PeerInfo struct {
	ID          string   `json:"id"`
	Addrs       []string `json:"addrs"`
	ConnectedAt int64    `json:"connected_at,omitempty"`
}

// PeersResponse represents the response for peer list endpoint
type PeersResponse struct {
	Peers []PeerInfo `json:"peers"`
}

// handleGetPeers returns the list of connected peers
func (s *Server) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := s.validator.GetPeers()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to get peers: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Message: "Peers retrieved successfully",
		Data:    PeersResponse{Peers: peers},
	})
}

// handleGetConfig returns the current node configuration
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.validator.GetConfig()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to get config: "+err.Error())
		return
	}

	s.sendSuccess(w, "Config retrieved successfully", config)
}
