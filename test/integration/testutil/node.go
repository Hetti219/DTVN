package testutil

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Hetti219/DTVN/pkg/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestNode represents a validator node under test
type TestNode struct {
	ID              string
	Index           int
	APIPort         int
	P2PPort         int
	DataDir         string
	LogFile         string
	BootstrapAddr   string
	IsByzantine     bool
	IsBootstrap     bool
	TotalNodes      int

	cmd             *exec.Cmd
	ctx             context.Context
	cancel          context.CancelFunc
	started         bool
	healthy         bool
	stdout          *os.File
	stderr          *os.File
	mu              sync.RWMutex

	httpClient      *http.Client
}

// NodeConfig holds configuration for a test node
type NodeConfig struct {
	Index           int
	APIPort         int
	P2PPort         int
	DataDir         string
	LogsDir         string
	BootstrapAddr   string
	IsByzantine     bool
	IsBootstrap     bool
	TotalNodes      int
	ValidatorBinary string
}

// GetPeerIDForNode returns the deterministic peer ID for a given node ID
func GetPeerIDForNode(nodeID string) (peer.ID, error) {
	privKey, err := network.GenerateDeterministicKey(nodeID)
	if err != nil {
		return "", fmt.Errorf("failed to generate key for node %s: %w", nodeID, err)
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to get peer ID: %w", err)
	}
	return peerID, nil
}

// NewTestNode creates a new test node
func NewTestNode(cfg *NodeConfig) (*TestNode, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Use "node0", "node1" format to match validator's expected node ID format
	nodeID := fmt.Sprintf("node%d", cfg.Index)
	logFile := filepath.Join(cfg.LogsDir, fmt.Sprintf("%s.log", nodeID))
	dataDir := filepath.Join(cfg.DataDir, nodeID)

	// Create directories
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}
	if err := os.MkdirAll(cfg.LogsDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create logs dir: %w", err)
	}

	return &TestNode{
		ID:            nodeID,
		Index:         cfg.Index,
		APIPort:       cfg.APIPort,
		P2PPort:       cfg.P2PPort,
		DataDir:       dataDir,
		LogFile:       logFile,
		BootstrapAddr: cfg.BootstrapAddr,
		IsByzantine:   cfg.IsByzantine,
		IsBootstrap:   cfg.IsBootstrap,
		TotalNodes:    cfg.TotalNodes,
		ctx:           ctx,
		cancel:        cancel,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Start starts the validator node
func (n *TestNode) Start(validatorBinary string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.started {
		return fmt.Errorf("node %s already started", n.ID)
	}

	// Build command arguments (using single-dash flags as expected by validator)
	args := []string{
		"-id", n.ID,
		"-api-port", fmt.Sprintf("%d", n.APIPort),
		"-port", fmt.Sprintf("%d", n.P2PPort),
		"-data-dir", n.DataDir,
		"-total-nodes", fmt.Sprintf("%d", n.TotalNodes),
	}

	if n.IsBootstrap {
		args = append(args, "-bootstrap-node", "-primary")
	} else if n.BootstrapAddr != "" {
		args = append(args, "-bootstrap", n.BootstrapAddr)
	}

	// Note: Byzantine mode is simulated at test level, not by validator flag

	// Create command
	n.cmd = exec.CommandContext(n.ctx, validatorBinary, args...)

	// Setup logging
	logFile, err := os.Create(n.LogFile)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	n.stdout = logFile
	n.stderr = logFile
	n.cmd.Stdout = logFile
	n.cmd.Stderr = logFile

	// Start the process
	if err := n.cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start node %s: %w", n.ID, err)
	}

	n.started = true

	// Wait for node to be healthy
	go n.waitForHealth()

	return nil
}

// waitForHealth waits for the node to become healthy
func (n *TestNode) waitForHealth() {
	maxRetries := 30
	retryDelay := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		select {
		case <-n.ctx.Done():
			return
		case <-time.After(retryDelay):
			if n.checkHealth() {
				n.mu.Lock()
				n.healthy = true
				n.mu.Unlock()
				fmt.Printf("[%s] Node is healthy\n", n.ID)
				return
			}
		}
	}

	fmt.Printf("[%s] Node failed to become healthy after %d retries\n", n.ID, maxRetries)
}

// checkHealth checks if the node is healthy
func (n *TestNode) checkHealth() bool {
	url := fmt.Sprintf("http://localhost:%d/health", n.APIPort)
	resp, err := n.httpClient.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Stop stops the validator node
func (n *TestNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.started {
		return nil
	}

	// Cancel context to stop the process
	n.cancel()

	// Wait for process to exit
	if n.cmd != nil && n.cmd.Process != nil {
		if err := n.cmd.Wait(); err != nil && !strings.Contains(err.Error(), "signal: killed") {
			fmt.Printf("[%s] Error waiting for process: %v\n", n.ID, err)
		}
	}

	// Close log files
	if n.stdout != nil {
		n.stdout.Close()
	}
	if n.stderr != nil {
		n.stderr.Close()
	}

	n.started = false
	n.healthy = false

	return nil
}

// IsHealthy returns whether the node is healthy
func (n *TestNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}

// IsStarted returns whether the node is started
func (n *TestNode) IsStarted() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.started
}

// ValidateTicket validates a ticket via the API
func (n *TestNode) ValidateTicket(ticketID string, data []byte) error {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tickets/validate", n.APIPort)

	payload := map[string]interface{}{
		"ticket_id": ticketID,
		"data":      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := n.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("validation failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ConsumeTicket consumes a ticket via the API
func (n *TestNode) ConsumeTicket(ticketID string) error {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tickets/consume", n.APIPort)

	payload := map[string]interface{}{
		"ticket_id": ticketID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := n.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("consume failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DisputeTicket disputes a ticket via the API
func (n *TestNode) DisputeTicket(ticketID string) error {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tickets/dispute", n.APIPort)

	payload := map[string]interface{}{
		"ticket_id": ticketID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := n.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dispute failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetTicket retrieves a ticket via the API
func (n *TestNode) GetTicket(ticketID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tickets/%s", n.APIPort, ticketID)

	resp, err := n.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// GetAllTickets retrieves all tickets via the API
func (n *TestNode) GetAllTickets() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/tickets", n.APIPort)

	resp, err := n.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract tickets from data field
	if data, ok := result["data"].([]interface{}); ok {
		tickets := make([]map[string]interface{}, len(data))
		for i, item := range data {
			if ticket, ok := item.(map[string]interface{}); ok {
				tickets[i] = ticket
			}
		}
		return tickets, nil
	}

	return []map[string]interface{}{}, nil
}

// GetStats retrieves node stats via the API
func (n *TestNode) GetStats() (map[string]interface{}, error) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/stats", n.APIPort)

	resp, err := n.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		return data, nil
	}

	return map[string]interface{}{}, nil
}

// GetPeers retrieves peer information via the API
func (n *TestNode) GetPeers() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/peers", n.APIPort)

	resp, err := n.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if data, ok := result["data"].([]interface{}); ok {
		peers := make([]map[string]interface{}, len(data))
		for i, item := range data {
			if peer, ok := item.(map[string]interface{}); ok {
				peers[i] = peer
			}
		}
		return peers, nil
	}

	return []map[string]interface{}{}, nil
}

// GetLogs retrieves the last N lines from the node's log file
func (n *TestNode) GetLogs(lines int) ([]string, error) {
	file, err := os.Open(n.LogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var logLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		logLines = append(logLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	// Return last N lines
	if len(logLines) > lines {
		return logLines[len(logLines)-lines:], nil
	}

	return logLines, nil
}

// Cleanup removes data and log files
func (n *TestNode) Cleanup() error {
	if err := os.RemoveAll(n.DataDir); err != nil {
		return fmt.Errorf("failed to remove data dir: %w", err)
	}
	if err := os.Remove(n.LogFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove log file: %w", err)
	}
	return nil
}
