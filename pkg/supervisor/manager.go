package supervisor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// NodeStatus represents the status of a managed node
type NodeStatus string

const (
	NodeStatusStopped  NodeStatus = "stopped"
	NodeStatusStarting NodeStatus = "starting"
	NodeStatusRunning  NodeStatus = "running"
	NodeStatusStopping NodeStatus = "stopping"
	NodeStatusError    NodeStatus = "error"
)

// ManagedNode represents a validator node managed by the supervisor
type ManagedNode struct {
	ID          string            `json:"id"`
	Port        int               `json:"port"`
	APIPort     int               `json:"api_port"`
	Status      NodeStatus        `json:"status"`
	PeerID      string            `json:"peer_id,omitempty"`
	IsPrimary   bool              `json:"is_primary"`
	IsBootstrap bool              `json:"is_bootstrap"`
	StartedAt   time.Time         `json:"started_at,omitempty"`
	Error       string            `json:"error,omitempty"`
	Config      map[string]string `json:"config,omitempty"`

	cmd        *exec.Cmd
	cancelFunc context.CancelFunc
	outputBuf  *OutputBuffer
	mu         sync.RWMutex
}

// OutputBuffer is a thread-safe circular buffer for output
type OutputBuffer struct {
	lines    []string
	maxLines int
	mu       sync.RWMutex
}

// NewOutputBuffer creates a new output buffer
func NewOutputBuffer(maxLines int) *OutputBuffer {
	return &OutputBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

// Write adds a line to the buffer
func (b *OutputBuffer) Write(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines = append(b.lines, line)
	if len(b.lines) > b.maxLines {
		b.lines = b.lines[len(b.lines)-b.maxLines:]
	}
}

// GetLines returns all lines in the buffer
func (b *OutputBuffer) GetLines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, len(b.lines))
	copy(result, b.lines)
	return result
}

// Clear clears the buffer
func (b *OutputBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = b.lines[:0]
}

// NodeManager manages multiple validator node processes
type NodeManager struct {
	nodes          map[string]*ManagedNode
	validatorPath  string
	dataDir        string
	basePort       int
	baseAPIPort    int
	totalNodes     int
	mu             sync.RWMutex
	outputCallback func(nodeID string, line string)
	statusCallback func(nodeID string, status NodeStatus)
}

// NodeManagerConfig holds configuration for the node manager
type NodeManagerConfig struct {
	ValidatorPath  string
	DataDir        string
	BasePort       int
	BaseAPIPort    int
	OutputCallback func(nodeID string, line string)
	StatusCallback func(nodeID string, status NodeStatus)
}

// NewNodeManager creates a new node manager
func NewNodeManager(cfg *NodeManagerConfig) *NodeManager {
	if cfg.BasePort == 0 {
		cfg.BasePort = 4001
	}
	if cfg.BaseAPIPort == 0 {
		cfg.BaseAPIPort = 9001
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	return &NodeManager{
		nodes:          make(map[string]*ManagedNode),
		validatorPath:  cfg.ValidatorPath,
		dataDir:        cfg.DataDir,
		basePort:       cfg.BasePort,
		baseAPIPort:    cfg.BaseAPIPort,
		outputCallback: cfg.OutputCallback,
		statusCallback: cfg.StatusCallback,
	}
}

// StartNode starts a single validator node
func (m *NodeManager) StartNode(nodeID string, isPrimary bool, isBootstrap bool, bootstrapAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if node already exists and is running
	if existing, ok := m.nodes[nodeID]; ok {
		if existing.Status == NodeStatusRunning || existing.Status == NodeStatusStarting {
			return fmt.Errorf("node %s is already running", nodeID)
		}
	}

	// Calculate ports based on node index
	nodeIndex := len(m.nodes)
	if existing, ok := m.nodes[nodeID]; ok {
		// Reuse existing ports
		nodeIndex = existing.Port - m.basePort
	}

	port := m.basePort + nodeIndex
	apiPort := m.baseAPIPort + nodeIndex

	// Create node entry
	node := &ManagedNode{
		ID:          nodeID,
		Port:        port,
		APIPort:     apiPort,
		Status:      NodeStatusStarting,
		IsPrimary:   isPrimary,
		IsBootstrap: isBootstrap,
		outputBuf:   NewOutputBuffer(1000),
	}
	m.nodes[nodeID] = node
	m.totalNodes = len(m.nodes)

	// Start the node process asynchronously
	go m.startNodeProcess(node, bootstrapAddr)

	return nil
}

// startNodeProcess starts the actual validator process
func (m *NodeManager) startNodeProcess(node *ManagedNode, bootstrapAddr string) {
	ctx, cancel := context.WithCancel(context.Background())
	node.mu.Lock()
	node.cancelFunc = cancel
	node.mu.Unlock()

	// Build command arguments
	args := []string{
		"-id", node.ID,
		"-port", fmt.Sprintf("%d", node.Port),
		"-api-port", fmt.Sprintf("%d", node.APIPort),
		"-data-dir", filepath.Join(m.dataDir, node.ID),
		"-total-nodes", fmt.Sprintf("%d", m.totalNodes),
	}

	if node.IsPrimary {
		args = append(args, "-primary")
	}

	if node.IsBootstrap {
		args = append(args, "-bootstrap-node")
	}

	if bootstrapAddr != "" {
		args = append(args, "-bootstrap", bootstrapAddr)
	}

	// Create command
	cmd := exec.CommandContext(ctx, m.validatorPath, args...)
	cmd.Dir = filepath.Dir(m.validatorPath)
	if cmd.Dir == "" || cmd.Dir == "." {
		// If validator path is relative, use current working directory
		if cwd, err := os.Getwd(); err == nil {
			cmd.Dir = cwd
		}
	}

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.setNodeError(node, fmt.Sprintf("failed to create stdout pipe: %v", err))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.setNodeError(node, fmt.Sprintf("failed to create stderr pipe: %v", err))
		return
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		m.setNodeError(node, fmt.Sprintf("failed to start process: %v", err))
		return
	}

	node.mu.Lock()
	node.cmd = cmd
	node.Status = NodeStatusRunning
	node.StartedAt = time.Now()
	node.mu.Unlock()

	m.notifyStatus(node.ID, NodeStatusRunning)

	// Stream output
	go m.streamOutput(node, stdout, "stdout")
	go m.streamOutput(node, stderr, "stderr")

	// Wait for process to exit
	err = cmd.Wait()

	node.mu.Lock()
	if ctx.Err() == context.Canceled {
		// Normal shutdown
		node.Status = NodeStatusStopped
	} else if err != nil {
		node.Status = NodeStatusError
		node.Error = fmt.Sprintf("process exited with error: %v", err)
	} else {
		node.Status = NodeStatusStopped
	}
	node.mu.Unlock()

	m.notifyStatus(node.ID, node.Status)
}

// streamOutput streams output from a reader to the node's buffer
func (m *NodeManager) streamOutput(node *ManagedNode, reader io.Reader, source string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		node.outputBuf.Write(fmt.Sprintf("[%s] %s", source, line))

		if m.outputCallback != nil {
			m.outputCallback(node.ID, line)
		}
	}
}

// setNodeError sets an error status on a node
func (m *NodeManager) setNodeError(node *ManagedNode, errMsg string) {
	node.mu.Lock()
	node.Status = NodeStatusError
	node.Error = errMsg
	node.mu.Unlock()

	m.notifyStatus(node.ID, NodeStatusError)
}

// notifyStatus notifies status change via callback
func (m *NodeManager) notifyStatus(nodeID string, status NodeStatus) {
	if m.statusCallback != nil {
		m.statusCallback(nodeID, status)
	}
}

// StopNode stops a single validator node
func (m *NodeManager) StopNode(nodeID string) error {
	m.mu.RLock()
	node, ok := m.nodes[nodeID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.Lock()
	if node.Status != NodeStatusRunning {
		node.mu.Unlock()
		return fmt.Errorf("node %s is not running (status: %s)", nodeID, node.Status)
	}

	node.Status = NodeStatusStopping
	cancelFunc := node.cancelFunc
	node.mu.Unlock()

	m.notifyStatus(nodeID, NodeStatusStopping)

	if cancelFunc != nil {
		cancelFunc()
	}

	return nil
}

// StopAllNodes stops all running nodes
func (m *NodeManager) StopAllNodes() {
	m.mu.RLock()
	nodeIDs := make([]string, 0, len(m.nodes))
	for id := range m.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	m.mu.RUnlock()

	for _, id := range nodeIDs {
		m.StopNode(id)
	}
}

// GetNode returns information about a specific node
func (m *NodeManager) GetNode(nodeID string) (*ManagedNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.RLock()
	defer node.mu.RUnlock()

	// Return a copy
	return &ManagedNode{
		ID:          node.ID,
		Port:        node.Port,
		APIPort:     node.APIPort,
		Status:      node.Status,
		PeerID:      node.PeerID,
		IsPrimary:   node.IsPrimary,
		IsBootstrap: node.IsBootstrap,
		StartedAt:   node.StartedAt,
		Error:       node.Error,
	}, nil
}

// GetAllNodes returns information about all nodes
func (m *NodeManager) GetAllNodes() []*ManagedNode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]*ManagedNode, 0, len(m.nodes))
	for _, node := range m.nodes {
		node.mu.RLock()
		nodes = append(nodes, &ManagedNode{
			ID:          node.ID,
			Port:        node.Port,
			APIPort:     node.APIPort,
			Status:      node.Status,
			PeerID:      node.PeerID,
			IsPrimary:   node.IsPrimary,
			IsBootstrap: node.IsBootstrap,
			StartedAt:   node.StartedAt,
			Error:       node.Error,
		})
		node.mu.RUnlock()
	}

	return nodes
}

// GetNodeLogs returns the output buffer for a node
func (m *NodeManager) GetNodeLogs(nodeID string) ([]string, error) {
	m.mu.RLock()
	node, ok := m.nodes[nodeID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node.outputBuf.GetLines(), nil
}

// StartCluster starts a cluster of nodes
func (m *NodeManager) StartCluster(nodeCount int) error {
	if nodeCount < 1 {
		return fmt.Errorf("node count must be at least 1")
	}

	// Start bootstrap node first
	bootstrapID := "node0"
	if err := m.StartNode(bootstrapID, true, true, ""); err != nil {
		return fmt.Errorf("failed to start bootstrap node: %w", err)
	}

	// Wait for bootstrap node to start
	time.Sleep(2 * time.Second)

	// Get bootstrap address
	m.mu.RLock()
	bootstrapNode := m.nodes[bootstrapID]
	m.mu.RUnlock()

	if bootstrapNode == nil {
		return fmt.Errorf("bootstrap node not found after starting")
	}

	// Construct bootstrap address (nodes will need to discover the peer ID)
	bootstrapAddr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", bootstrapNode.Port)

	// Start remaining nodes
	for i := 1; i < nodeCount; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		if err := m.StartNode(nodeID, false, false, bootstrapAddr); err != nil {
			return fmt.Errorf("failed to start node %s: %w", nodeID, err)
		}
		// Small delay between node starts
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// RemoveNode removes a node from management (must be stopped first)
func (m *NodeManager) RemoveNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	node, ok := m.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	node.mu.RLock()
	status := node.Status
	node.mu.RUnlock()

	if status == NodeStatusRunning || status == NodeStatusStarting {
		return fmt.Errorf("cannot remove running node %s", nodeID)
	}

	delete(m.nodes, nodeID)
	m.totalNodes = len(m.nodes)
	return nil
}

// SetTotalNodes sets the total node count for consensus
func (m *NodeManager) SetTotalNodes(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalNodes = count
}
