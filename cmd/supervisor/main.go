package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Hetti219/DTVN/pkg/supervisor"
)

func main() {
	// Parse command line flags
	webPort := flag.Int("web-port", 8080, "Web interface port")
	webAddr := flag.String("web-addr", "0.0.0.0", "Web interface bind address")
	dataDir := flag.String("data-dir", "./data", "Data directory for nodes")
	validatorPath := flag.String("validator", "", "Path to validator binary (auto-detected if empty)")
	simulatorPath := flag.String("simulator", "", "Path to simulator binary (auto-detected if empty)")
	staticDir := flag.String("static-dir", "", "Path to static files directory (auto-detected if empty)")
	autoStart := flag.Int("auto-start", 0, "Auto-start this many nodes on startup (0 = disabled)")
	flag.Parse()

	fmt.Println("=== DTVN Supervisor ===")
	fmt.Println("Distributed Ticket Validation Network - Web Interface")
	fmt.Println()

	// Auto-detect binary paths if not specified
	if *validatorPath == "" {
		*validatorPath = findBinary("validator")
	}
	if *simulatorPath == "" {
		*simulatorPath = findBinary("simulator")
	}

	// Verify binaries exist
	if *validatorPath == "" {
		fmt.Println("ERROR: Validator binary not found")
		fmt.Println("Please build it with: go build -o bin/validator cmd/validator/main.go")
		fmt.Println("Or specify the path with: -validator /path/to/validator")
		os.Exit(1)
	}

	if *simulatorPath == "" {
		fmt.Println("WARNING: Simulator binary not found - simulation features disabled")
		fmt.Println("Build it with: go build -o bin/simulator cmd/simulator/main.go")
	}

	fmt.Printf("Validator binary: %s\n", *validatorPath)
	if *simulatorPath != "" {
		fmt.Printf("Simulator binary: %s\n", *simulatorPath)
	}
	fmt.Printf("Data directory: %s\n", *dataDir)
	fmt.Println()

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		fmt.Printf("ERROR: Failed to create data directory: %v\n", err)
		os.Exit(1)
	}

	// Create supervisor server
	server := supervisor.NewServer(&supervisor.ServerConfig{
		Address:       *webAddr,
		Port:          *webPort,
		ValidatorPath: *validatorPath,
		SimulatorPath: *simulatorPath,
		DataDir:       *dataDir,
		StaticDir:     *staticDir,
	})

	// Auto-start cluster if requested
	if *autoStart > 0 {
		fmt.Printf("Auto-starting cluster with %d nodes...\n", *autoStart)
		go func() {
			// Wait a moment for server to start
			time.Sleep(500 * time.Millisecond)
			// Start cluster through internal API (we'd need to expose this)
		}()
	}

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Printf("\nSupervisor running on http://%s:%d\n", *webAddr, *webPort)
	fmt.Println("\nOpen this URL in your browser to access the web interface.")
	fmt.Println("Press Ctrl+C to stop the supervisor.")
	fmt.Println()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down supervisor...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}

	fmt.Println("Supervisor stopped")
}

// findBinary tries to find a binary in common locations
func findBinary(name string) string {
	// Check common locations
	locations := []string{
		filepath.Join("bin", name),
		filepath.Join(".", name),
		name,
	}

	// Also check relative to executable
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		locations = append(locations,
			filepath.Join(execDir, name),
			filepath.Join(execDir, "..", "bin", name),
		)
	}

	// Check current working directory
	if cwd, err := os.Getwd(); err == nil {
		locations = append(locations,
			filepath.Join(cwd, "bin", name),
			filepath.Join(cwd, name),
		)
	}

	for _, loc := range locations {
		if absPath, err := filepath.Abs(loc); err == nil {
			if _, err := os.Stat(absPath); err == nil {
				return absPath
			}
		}
	}

	return ""
}
