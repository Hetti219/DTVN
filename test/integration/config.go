package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// projectRoot returns the absolute path to the project root directory.
// It walks up from the current source file until it finds go.mod.
func projectRoot() string {
	// First check environment variable (set by the test runner script)
	if root := os.Getenv("DTVN_PROJECT_ROOT"); root != "" {
		return root
	}

	// Derive from this source file's location:
	// this file is at <root>/test/integration/config.go
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(filename), "..", "..")
	}

	// Last resort: current working directory
	cwd, _ := os.Getwd()
	return cwd
}

// TestConfig defines the configuration for integration tests
type TestConfig struct {
	// Network configuration
	NumNodes       int   `json:"num_nodes"`
	ByzantineNodes []int `json:"byzantine_nodes"` // Indices of Byzantine nodes
	BootstrapPort  int   `json:"bootstrap_port"`
	APIPortStart   int   `json:"api_port_start"`
	P2PPortStart   int   `json:"p2p_port_start"`

	// Test behavior
	TestTimeout       time.Duration `json:"test_timeout"`
	NodeStartupDelay  time.Duration `json:"node_startup_delay"`
	ConsensusWaitTime time.Duration `json:"consensus_wait_time"`

	// Network conditions
	NetworkLatency   time.Duration `json:"network_latency"`
	PacketLossRate   float64       `json:"packet_loss_rate"`
	EnablePartitions bool          `json:"enable_partitions"`

	// Test data
	TicketPrefix string `json:"ticket_prefix"`

	// Directories
	DataDir         string `json:"data_dir"`
	LogsDir         string `json:"logs_dir"`
	ValidatorBinary string `json:"validator_binary"`

	// Assertions
	RequiredConsensus float64       `json:"required_consensus"` // 0.0-1.0 (e.g., 0.66 for 2f+1)
	MaxLatency        time.Duration `json:"max_latency"`
	MaxP95Latency     time.Duration `json:"max_p95_latency"`
}

// DefaultTestConfig returns a default test configuration
func DefaultTestConfig() *TestConfig {
	root := projectRoot()
	return &TestConfig{
		NumNodes:          9,
		ByzantineNodes:    []int{},
		BootstrapPort:     14001,
		APIPortStart:      18000,
		P2PPortStart:      14000,
		TestTimeout:       5 * time.Minute,
		NodeStartupDelay:  2 * time.Second,
		ConsensusWaitTime: 5 * time.Second,
		NetworkLatency:    50 * time.Millisecond,
		PacketLossRate:    0.0,
		EnablePartitions:  false,
		TicketPrefix:      "TEST-TICKET",
		DataDir:           filepath.Join(root, "test-data", "integration"),
		LogsDir:           filepath.Join(root, "test-data", "logs"),
		ValidatorBinary:   filepath.Join(root, "bin", "validator"),
		RequiredConsensus: 0.66,
		MaxLatency:        1000 * time.Millisecond,
		MaxP95Latency:     2000 * time.Millisecond,
	}
}

// ByzantineTestConfig returns configuration for Byzantine testing
func ByzantineTestConfig() *TestConfig {
	cfg := DefaultTestConfig()
	cfg.NumNodes = 10
	cfg.ByzantineNodes = []int{7, 8, 9} // Last 3 nodes are Byzantine
	// Multiple tickets are processed sequentially through PBFT on the primary.
	// Each round takes ~1-2s on a 10-node network, so allow sufficient time
	// for all rounds to complete and state to replicate.
	cfg.ConsensusWaitTime = 10 * time.Second
	return cfg
}

// PartitionTestConfig returns configuration for partition testing
func PartitionTestConfig() *TestConfig {
	cfg := DefaultTestConfig()
	cfg.EnablePartitions = true
	cfg.ConsensusWaitTime = 10 * time.Second
	return cfg
}

// HighLatencyTestConfig returns configuration for high latency testing
func HighLatencyTestConfig() *TestConfig {
	cfg := DefaultTestConfig()
	cfg.NetworkLatency = 500 * time.Millisecond
	cfg.MaxLatency = 3000 * time.Millisecond
	cfg.MaxP95Latency = 5000 * time.Millisecond
	return cfg
}

// PacketLossTestConfig returns configuration for packet loss testing
func PacketLossTestConfig() *TestConfig {
	cfg := DefaultTestConfig()
	cfg.PacketLossRate = 0.05 // 5% packet loss
	cfg.ConsensusWaitTime = 8 * time.Second
	return cfg
}

// TestResult holds the results of a test run
type TestResult struct {
	Scenario     string
	Success      bool
	Duration     time.Duration
	ErrorMessage string
	Metrics      *TestMetrics
	NodeResults  map[string]*NodeTestResult
}

// TestMetrics holds test metrics
type TestMetrics struct {
	TotalTickets      int
	ValidatedTickets  int
	FailedValidations int
	ConsensusRounds   int
	AverageLatency    time.Duration
	P50Latency        time.Duration
	P95Latency        time.Duration
	P99Latency        time.Duration
	MessagesSent      int64
	MessagesReceived  int64
	StateConsistency  float64 // 0.0-1.0
}

// NodeTestResult holds per-node test results
type NodeTestResult struct {
	NodeID           string
	Started          bool
	Healthy          bool
	TicketsSeen      int
	TicketsValidated int
	ConsensusRounds  int
	ErrorCount       int
	LastError        string
}
