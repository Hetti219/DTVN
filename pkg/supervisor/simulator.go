package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SimulatorStatus represents the status of the simulator
type SimulatorStatus string

const (
	SimulatorStatusIdle     SimulatorStatus = "idle"
	SimulatorStatusRunning  SimulatorStatus = "running"
	SimulatorStatusComplete SimulatorStatus = "complete"
	SimulatorStatusError    SimulatorStatus = "error"
)

// SimulatorConfig holds simulator configuration
type SimulatorConfig struct {
	Nodes            int     `json:"nodes"`
	ByzantineNodes   int     `json:"byzantine_nodes"`
	Tickets          int     `json:"tickets"`
	DurationSeconds  int     `json:"duration_seconds"`
	LatencyMs        int     `json:"latency_ms"`
	PacketLossRate   float64 `json:"packet_loss_rate"`
	EnablePartitions bool    `json:"enable_partitions"`
}

// SimulatorResults holds simulation results
type SimulatorResults struct {
	TotalTickets      int     `json:"total_tickets"`
	ValidatedTickets  int     `json:"validated_tickets"`
	ConsensusRounds   int     `json:"consensus_rounds"`
	SuccessfulRounds  int     `json:"successful_rounds"`
	FailedRounds      int     `json:"failed_rounds"`
	SuccessRate       float64 `json:"success_rate"`
	MessagesSent      int64   `json:"messages_sent"`
	MessagesReceived  int64   `json:"messages_received"`
	PartitionEvents   int     `json:"partition_events"`
	AverageLatencyMs  float64 `json:"average_latency_ms"`
}

// SimulatorProgress holds current progress
type SimulatorProgress struct {
	CurrentTicket    int     `json:"current_ticket"`
	TotalTickets     int     `json:"total_tickets"`
	ConsensusRounds  int     `json:"consensus_rounds"`
	SuccessfulRounds int     `json:"successful_rounds"`
	FailedRounds     int     `json:"failed_rounds"`
	ProgressPercent  float64 `json:"progress_percent"`
}

// SimulatorController manages simulator subprocess
type SimulatorController struct {
	simulatorPath    string
	status           SimulatorStatus
	config           *SimulatorConfig
	progress         *SimulatorProgress
	results          *SimulatorResults
	outputLines      []string
	maxOutputLines   int
	cmd              *exec.Cmd
	cancelFunc       context.CancelFunc
	mu               sync.RWMutex
	outputCallback   func(line string)
	progressCallback func(progress *SimulatorProgress)
	statusCallback   func(status SimulatorStatus)
	resultCallback   func(results *SimulatorResults)
}

// SimulatorControllerConfig holds controller configuration
type SimulatorControllerConfig struct {
	SimulatorPath    string
	MaxOutputLines   int
	OutputCallback   func(line string)
	ProgressCallback func(progress *SimulatorProgress)
	StatusCallback   func(status SimulatorStatus)
	ResultCallback   func(results *SimulatorResults)
}

// NewSimulatorController creates a new simulator controller
func NewSimulatorController(cfg *SimulatorControllerConfig) *SimulatorController {
	maxLines := cfg.MaxOutputLines
	if maxLines == 0 {
		maxLines = 500
	}

	return &SimulatorController{
		simulatorPath:    cfg.SimulatorPath,
		status:           SimulatorStatusIdle,
		outputLines:      make([]string, 0, maxLines),
		maxOutputLines:   maxLines,
		outputCallback:   cfg.OutputCallback,
		progressCallback: cfg.ProgressCallback,
		statusCallback:   cfg.StatusCallback,
		resultCallback:   cfg.ResultCallback,
	}
}

// Start starts the simulator with the given configuration
func (s *SimulatorController) Start(config *SimulatorConfig) error {
	s.mu.Lock()
	if s.status == SimulatorStatusRunning {
		s.mu.Unlock()
		return fmt.Errorf("simulator is already running")
	}

	s.status = SimulatorStatusRunning
	s.config = config
	s.progress = &SimulatorProgress{
		TotalTickets: config.Tickets,
	}
	s.results = nil
	s.outputLines = s.outputLines[:0]
	s.mu.Unlock()

	s.notifyStatus(SimulatorStatusRunning)

	go s.runSimulator(config)
	return nil
}

// runSimulator runs the simulator subprocess
func (s *SimulatorController) runSimulator(config *SimulatorConfig) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancelFunc = cancel
	s.mu.Unlock()

	// Build command arguments
	args := []string{
		"-nodes", fmt.Sprintf("%d", config.Nodes),
		"-byzantine", fmt.Sprintf("%d", config.ByzantineNodes),
		"-tickets", fmt.Sprintf("%d", config.Tickets),
		"-duration", fmt.Sprintf("%ds", config.DurationSeconds),
		"-latency", fmt.Sprintf("%dms", config.LatencyMs),
		"-packet-loss", fmt.Sprintf("%.4f", config.PacketLossRate),
	}

	if config.EnablePartitions {
		args = append(args, "-partition")
	}

	// Create command
	cmd := exec.CommandContext(ctx, s.simulatorPath, args...)

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.setError(fmt.Sprintf("failed to create stdout pipe: %v", err))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.setError(fmt.Sprintf("failed to create stderr pipe: %v", err))
		return
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		s.setError(fmt.Sprintf("failed to start simulator: %v", err))
		return
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	// Stream output
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		s.streamOutput(stdout)
	}()

	go func() {
		defer wg.Done()
		s.streamOutput(stderr)
	}()

	// Wait for output streams to close
	wg.Wait()

	// Wait for process to exit
	err = cmd.Wait()

	s.mu.Lock()
	if ctx.Err() == context.Canceled {
		s.status = SimulatorStatusIdle
		s.addOutput("Simulation cancelled by user")
	} else if err != nil {
		s.status = SimulatorStatusError
		s.addOutput(fmt.Sprintf("Simulation error: %v", err))
	} else {
		s.status = SimulatorStatusComplete
		s.addOutput("Simulation completed successfully")
	}
	status := s.status
	s.mu.Unlock()

	s.notifyStatus(status)
}

// streamOutput streams output from the simulator
func (s *SimulatorController) streamOutput(reader io.Reader) {
	scanner := bufio.NewScanner(reader)

	// Regex patterns for parsing output
	progressRegex := regexp.MustCompile(`Progress: (\d+)/(\d+) tickets \| Rounds: (\d+) \| Success: (\d+) \| Failed: (\d+)`)
	resultRegex := regexp.MustCompile(`(\w+[\w\s]*): ([\d.]+%?)`)

	for scanner.Scan() {
		line := scanner.Text()

		s.mu.Lock()
		s.addOutput(line)
		s.mu.Unlock()

		// Parse progress updates
		if matches := progressRegex.FindStringSubmatch(line); len(matches) == 6 {
			current, _ := strconv.Atoi(matches[1])
			total, _ := strconv.Atoi(matches[2])
			rounds, _ := strconv.Atoi(matches[3])
			success, _ := strconv.Atoi(matches[4])
			failed, _ := strconv.Atoi(matches[5])

			progress := &SimulatorProgress{
				CurrentTicket:    current,
				TotalTickets:     total,
				ConsensusRounds:  rounds,
				SuccessfulRounds: success,
				FailedRounds:     failed,
				ProgressPercent:  float64(current) / float64(total) * 100,
			}

			s.mu.Lock()
			s.progress = progress
			s.mu.Unlock()

			s.notifyProgress(progress)
		}

		// Parse results section
		if strings.Contains(line, "=== Simulation Results ===") {
			s.mu.Lock()
			if s.results == nil {
				s.results = &SimulatorResults{}
			}
			s.mu.Unlock()
		}

		// Parse individual result lines
		if matches := resultRegex.FindStringSubmatch(line); len(matches) == 3 {
			s.parseResultLine(matches[1], matches[2])
		}

		// Notify output callback
		if s.outputCallback != nil {
			s.outputCallback(line)
		}
	}
}

// parseResultLine parses a result line
func (s *SimulatorController) parseResultLine(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.results == nil {
		s.results = &SimulatorResults{}
	}

	// Clean up the value
	value = strings.TrimSuffix(value, "%")
	numValue, _ := strconv.ParseFloat(value, 64)
	intValue := int(numValue)

	switch strings.TrimSpace(key) {
	case "Total Tickets":
		s.results.TotalTickets = intValue
	case "Validated Tickets":
		s.results.ValidatedTickets = intValue
	case "Consensus Rounds":
		s.results.ConsensusRounds = intValue
	case "Successful Rounds":
		s.results.SuccessfulRounds = intValue
	case "Failed Rounds":
		s.results.FailedRounds = intValue
	case "Success Rate":
		s.results.SuccessRate = numValue
	case "Messages Sent":
		s.results.MessagesSent = int64(intValue)
	case "Messages Received":
		s.results.MessagesReceived = int64(intValue)
	case "Partition Events":
		s.results.PartitionEvents = intValue
	}
}

// addOutput adds a line to the output buffer (must be called with lock held)
func (s *SimulatorController) addOutput(line string) {
	s.outputLines = append(s.outputLines, line)
	if len(s.outputLines) > s.maxOutputLines {
		s.outputLines = s.outputLines[len(s.outputLines)-s.maxOutputLines:]
	}
}

// setError sets error status
func (s *SimulatorController) setError(errMsg string) {
	s.mu.Lock()
	s.status = SimulatorStatusError
	s.addOutput(fmt.Sprintf("ERROR: %s", errMsg))
	s.mu.Unlock()

	s.notifyStatus(SimulatorStatusError)
}

// notifyStatus notifies status change
func (s *SimulatorController) notifyStatus(status SimulatorStatus) {
	if s.statusCallback != nil {
		s.statusCallback(status)
	}
}

// notifyProgress notifies progress update
func (s *SimulatorController) notifyProgress(progress *SimulatorProgress) {
	if s.progressCallback != nil {
		s.progressCallback(progress)
	}
}

// Stop stops the running simulation
func (s *SimulatorController) Stop() error {
	s.mu.Lock()
	if s.status != SimulatorStatusRunning {
		s.mu.Unlock()
		return fmt.Errorf("simulator is not running")
	}

	cancelFunc := s.cancelFunc
	s.mu.Unlock()

	if cancelFunc != nil {
		cancelFunc()
	}

	return nil
}

// GetStatus returns the current simulator status
func (s *SimulatorController) GetStatus() SimulatorStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetProgress returns the current progress
func (s *SimulatorController) GetProgress() *SimulatorProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return nil
	}

	// Return a copy
	return &SimulatorProgress{
		CurrentTicket:    s.progress.CurrentTicket,
		TotalTickets:     s.progress.TotalTickets,
		ConsensusRounds:  s.progress.ConsensusRounds,
		SuccessfulRounds: s.progress.SuccessfulRounds,
		FailedRounds:     s.progress.FailedRounds,
		ProgressPercent:  s.progress.ProgressPercent,
	}
}

// GetResults returns the simulation results
func (s *SimulatorController) GetResults() *SimulatorResults {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.results == nil {
		return nil
	}

	// Return a copy
	return &SimulatorResults{
		TotalTickets:     s.results.TotalTickets,
		ValidatedTickets: s.results.ValidatedTickets,
		ConsensusRounds:  s.results.ConsensusRounds,
		SuccessfulRounds: s.results.SuccessfulRounds,
		FailedRounds:     s.results.FailedRounds,
		SuccessRate:      s.results.SuccessRate,
		MessagesSent:     s.results.MessagesSent,
		MessagesReceived: s.results.MessagesReceived,
		PartitionEvents:  s.results.PartitionEvents,
	}
}

// GetOutput returns the output buffer
func (s *SimulatorController) GetOutput() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, len(s.outputLines))
	copy(result, s.outputLines)
	return result
}

// GetConfig returns the current configuration
func (s *SimulatorController) GetConfig() *SimulatorConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.config == nil {
		return nil
	}

	return &SimulatorConfig{
		Nodes:            s.config.Nodes,
		ByzantineNodes:   s.config.ByzantineNodes,
		Tickets:          s.config.Tickets,
		DurationSeconds:  s.config.DurationSeconds,
		LatencyMs:        s.config.LatencyMs,
		PacketLossRate:   s.config.PacketLossRate,
		EnablePartitions: s.config.EnablePartitions,
	}
}

// GetFullState returns the complete simulator state
func (s *SimulatorController) GetFullState() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := map[string]interface{}{
		"status": s.status,
		"output": s.outputLines,
	}

	if s.config != nil {
		state["config"] = s.config
	}

	if s.progress != nil {
		state["progress"] = s.progress
	}

	if s.results != nil {
		state["results"] = s.results
	}

	return state
}

// ToJSON returns the full state as JSON
func (s *SimulatorController) ToJSON() ([]byte, error) {
	return json.Marshal(s.GetFullState())
}

// DefaultSimulatorConfig returns default configuration
func DefaultSimulatorConfig() *SimulatorConfig {
	return &SimulatorConfig{
		Nodes:            7,
		ByzantineNodes:   2,
		Tickets:          100,
		DurationSeconds:  60,
		LatencyMs:        50,
		PacketLossRate:   0.01,
		EnablePartitions: false,
	}
}

// ValidateConfig validates the simulator configuration
func ValidateConfig(config *SimulatorConfig) error {
	if config.Nodes < 4 {
		return fmt.Errorf("minimum 4 nodes required for BFT")
	}

	if config.Nodes > 30 {
		return fmt.Errorf("maximum 30 nodes supported")
	}

	maxByzantine := (config.Nodes - 1) / 3
	if config.ByzantineNodes > maxByzantine {
		return fmt.Errorf("byzantine nodes (%d) exceeds maximum (%d) for %d total nodes",
			config.ByzantineNodes, maxByzantine, config.Nodes)
	}

	if config.Tickets < 1 {
		return fmt.Errorf("minimum 1 ticket required")
	}

	if config.Tickets > 1000 {
		return fmt.Errorf("maximum 1000 tickets supported")
	}

	if config.DurationSeconds < 10 {
		return fmt.Errorf("minimum 10 seconds duration required")
	}

	if config.DurationSeconds > 600 {
		return fmt.Errorf("maximum 600 seconds duration supported")
	}

	if config.PacketLossRate < 0 || config.PacketLossRate > 0.5 {
		return fmt.Errorf("packet loss rate must be between 0 and 0.5")
	}

	return nil
}
