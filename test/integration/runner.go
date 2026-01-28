package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Hetti219/DTVN/test/integration/testutil"
)

// TestScenario defines a test scenario
type TestScenario struct {
	Name        string
	Description string
	Config      *TestConfig
	TestFunc    func(*testutil.TestCluster) error
	Assertions  []Assertion
}

// Assertion defines a test assertion
type Assertion struct {
	Name     string
	Check    func(*testutil.TestCluster) error
	Critical bool // If true, test fails immediately on assertion failure
}

// TestRunner orchestrates integration test execution
type TestRunner struct {
	config   *TestConfig
	scenarios []TestScenario
	results  []TestResult
}

// NewTestRunner creates a new test runner
func NewTestRunner(config *TestConfig) *TestRunner {
	return &TestRunner{
		config:    config,
		scenarios: make([]TestScenario, 0),
		results:   make([]TestResult, 0),
	}
}

// AddScenario adds a test scenario to the runner
func (tr *TestRunner) AddScenario(scenario TestScenario) {
	tr.scenarios = append(tr.scenarios, scenario)
}

// RunAll runs all test scenarios
func (tr *TestRunner) RunAll() error {
	fmt.Printf("Running %d integration test scenarios\n", len(tr.scenarios))
	fmt.Println("═══════════════════════════════════════════════════════════")

	for i, scenario := range tr.scenarios {
		fmt.Printf("\n[%d/%d] Running: %s\n", i+1, len(tr.scenarios), scenario.Name)
		fmt.Printf("Description: %s\n", scenario.Description)

		result := tr.runScenario(scenario)
		tr.results = append(tr.results, result)

		if result.Success {
			fmt.Printf("✓ PASSED in %v\n", result.Duration)
		} else {
			fmt.Printf("✗ FAILED in %v: %s\n", result.Duration, result.ErrorMessage)
		}
	}

	return tr.printSummary()
}

// runScenario runs a single test scenario
func (tr *TestRunner) runScenario(scenario TestScenario) TestResult {
	startTime := time.Now()

	result := TestResult{
		Scenario:    scenario.Name,
		Success:     true,
		NodeResults: make(map[string]*NodeTestResult),
	}

	// Create test cluster
	clusterCfg := &testutil.ClusterConfig{
		NumNodes:        scenario.Config.NumNodes,
		ByzantineNodes:  scenario.Config.ByzantineNodes,
		BootstrapPort:   scenario.Config.BootstrapPort,
		APIPortStart:    scenario.Config.APIPortStart,
		P2PPortStart:    scenario.Config.P2PPortStart,
		DataDir:         scenario.Config.DataDir,
		LogsDir:         scenario.Config.LogsDir,
		ValidatorBinary: scenario.Config.ValidatorBinary,
	}

	cluster, err := testutil.NewTestCluster(clusterCfg)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to create cluster: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	defer func() {
		cluster.Stop()
		if result.Success {
			cluster.Cleanup()
		}
	}()

	// Start cluster
	if err := cluster.Start(); err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to start cluster: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Wait for nodes to be healthy
	time.Sleep(scenario.Config.NodeStartupDelay)

	// Run test function
	if err := scenario.TestFunc(cluster); err != nil {
		result.Success = false
		result.ErrorMessage = err.Error()
	}

	// Run assertions
	for _, assertion := range scenario.Assertions {
		if err := assertion.Check(cluster); err != nil {
			result.Success = false
			if assertion.Critical {
				result.ErrorMessage = fmt.Sprintf("Critical assertion failed: %s - %v",
					assertion.Name, err)
				result.Duration = time.Since(startTime)
				return result
			} else {
				if result.ErrorMessage == "" {
					result.ErrorMessage = fmt.Sprintf("Assertion failed: %s - %v",
						assertion.Name, err)
				}
			}
		}
	}

	// Collect metrics
	stats, _ := cluster.GetClusterStats()
	result.Metrics = &TestMetrics{
		TotalTickets: getIntStat(stats, "total_tickets"),
	}

	result.Duration = time.Since(startTime)
	return result
}

// printSummary prints test execution summary
func (tr *TestRunner) printSummary() error {
	fmt.Println("\n═══════════════════════════════════════════════════════════")
	fmt.Println("TEST SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════")

	passed := 0
	failed := 0
	totalDuration := time.Duration(0)

	for _, result := range tr.results {
		if result.Success {
			passed++
		} else {
			failed++
		}
		totalDuration += result.Duration
	}

	fmt.Printf("\nTotal Tests: %d\n", len(tr.results))
	fmt.Printf("✓ Passed: %d\n", passed)
	if failed > 0 {
		fmt.Printf("✗ Failed: %d\n", failed)
	}
	fmt.Printf("Total Duration: %v\n", totalDuration)

	// Print failed tests
	if failed > 0 {
		fmt.Println("\nFailed Tests:")
		for _, result := range tr.results {
			if !result.Success {
				fmt.Printf("  - %s: %s\n", result.Scenario, result.ErrorMessage)
			}
		}
	}

	fmt.Println("")

	if failed > 0 {
		return fmt.Errorf("%d test(s) failed", failed)
	}

	return nil
}

// SaveResults saves test results to a JSON file
func (tr *TestRunner) SaveResults(filename string) error {
	data, err := json.MarshalIndent(tr.results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write results file: %w", err)
	}

	return nil
}

// Helper functions

func getIntStat(stats map[string]interface{}, key string) int {
	if val, ok := stats[key].(int); ok {
		return val
	}
	if val, ok := stats[key].(float64); ok {
		return int(val)
	}
	return 0
}
