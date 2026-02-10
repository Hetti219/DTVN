.PHONY: all build clean test test-coverage test-unit test-integration integration-test run-validator run-simulator help

# Variables
BINARY_NAME=validator
SIMULATOR_NAME=simulator
SUPERVISOR_NAME=supervisor
GO=go

# Build all binaries
all: build

# Build validator, simulator, and supervisor
build:
	@echo "Building validator..."
	$(GO) build -o bin/$(BINARY_NAME) cmd/validator/main.go
	@echo "Building simulator..."
	$(GO) build -o bin/$(SIMULATOR_NAME) cmd/simulator/main.go
	@echo "Building supervisor..."
	$(GO) build -o bin/$(SUPERVISOR_NAME) cmd/supervisor/main.go
	@echo "Build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf data/
	rm -rf *.db
	@echo "Clean complete!"

# Run all tests (unit + integration)
test:
	@echo "Running all tests..."
	$(GO) test -v ./...

# Run unit tests only (fast)
test-unit:
	@echo "Running unit tests..."
	$(GO) test -v -short ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Run integration tests
test-integration: build
	@echo "Running integration tests..."
	@chmod +x scripts/run-integration-tests.sh
	@./scripts/run-integration-tests.sh

# Run integration tests with verbose output
test-integration-verbose: build
	@echo "Running integration tests (verbose)..."
	@chmod +x scripts/run-integration-tests.sh
	@./scripts/run-integration-tests.sh -v

# Run specific integration test
test-integration-single: build
	@echo "Running single integration test: $(TEST)"
	@chmod +x scripts/run-integration-tests.sh
	@./scripts/run-integration-tests.sh --pattern $(TEST)

# Run integration tests in parallel (faster)
test-integration-parallel: build
	@echo "Running integration tests in parallel..."
	@chmod +x scripts/run-integration-tests.sh
	@./scripts/run-integration-tests.sh -p

# Run integration tests and keep artifacts for debugging
test-integration-debug: build
	@echo "Running integration tests (debug mode)..."
	@chmod +x scripts/run-integration-tests.sh
	@./scripts/run-integration-tests.sh -v --no-cleanup

# Alias for backwards compatibility
integration-test: test-integration

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GO) mod tidy

# Generate protocol buffers (requires protoc)
proto:
	@echo "Generating protocol buffers..."
	protoc --go_out=. --go_opt=paths=source_relative proto/messages.proto

# Run a single validator node
run-validator:
	@echo "Starting validator node..."
	$(GO) run cmd/validator/main.go \
		-id node0 \
		-port 4001 \
		-api-port 8080 \
		-data-dir ./data/node0 \
		-primary \
		-total-nodes 4

# Run validator as bootstrap node
run-bootstrap:
	@echo "Starting bootstrap node..."
	$(GO) run cmd/validator/main.go \
		-id bootstrap \
		-port 4000 \
		-api-port 8000 \
		-data-dir ./data/bootstrap \
		-bootstrap-node \
		-primary \
		-total-nodes 7

# Deterministic peer ID for node0 (derived from node ID "node0")
NODE0_PEER_ID := 12D3KooWLtBkKrip2jyaRzhUphqYyVXGUPMMbmpBWHZMYXaueb9C

# Run a network of 4 validator nodes
run-network:
	@echo "Starting validator network..."
	@mkdir -p data/node0 data/node1 data/node2 data/node3
	$(GO) run cmd/validator/main.go -id node0 -port 4001 -api-port 8081 -data-dir ./data/node0 -primary -total-nodes 4 & \
	sleep 3 && \
	$(GO) run cmd/validator/main.go -id node1 -port 4002 -api-port 8082 -data-dir ./data/node1 -total-nodes 4 -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/$(NODE0_PEER_ID)" & \
	sleep 2 && \
	$(GO) run cmd/validator/main.go -id node2 -port 4003 -api-port 8083 -data-dir ./data/node2 -total-nodes 4 -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/$(NODE0_PEER_ID)" & \
	sleep 2 && \
	$(GO) run cmd/validator/main.go -id node3 -port 4004 -api-port 8084 -data-dir ./data/node3 -total-nodes 4 -bootstrap "/ip4/127.0.0.1/tcp/4001/p2p/$(NODE0_PEER_ID)"

# Run simulator
run-simulator:
	@echo "Starting simulator..."
	$(GO) run cmd/simulator/main.go \
		-nodes 7 \
		-byzantine 2 \
		-tickets 100 \
		-duration 60s \
		-latency 50ms \
		-packet-loss 0.01

# Run simulator with network partition
run-simulator-partition:
	@echo "Starting simulator with network partition..."
	$(GO) run cmd/simulator/main.go \
		-nodes 7 \
		-byzantine 2 \
		-tickets 100 \
		-duration 120s \
		-latency 100ms \
		-packet-loss 0.05 \
		-partition

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Development setup
dev-setup: deps
	@echo "Setting up development environment..."
	@mkdir -p data bin
	@echo "Development environment ready!"

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Clean:"
	@echo "  all                         - Build all binaries"
	@echo "  build                       - Build validator, simulator, and supervisor"
	@echo "  clean                       - Clean build artifacts"
	@echo ""
	@echo "Testing:"
	@echo "  test                        - Run all tests (unit + integration)"
	@echo "  test-unit                   - Run unit tests only (fast)"
	@echo "  test-coverage               - Run tests with coverage report"
	@echo "  test-integration            - Run integration tests"
	@echo "  test-integration-verbose    - Run integration tests (verbose)"
	@echo "  test-integration-parallel   - Run integration tests in parallel"
	@echo "  test-integration-debug      - Run integration tests (keep artifacts)"
	@echo "  test-integration-single     - Run single test (use TEST=name)"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint                        - Run linter"
	@echo "  fmt                         - Format code"
	@echo "  tidy                        - Tidy dependencies"
	@echo "  proto                       - Generate protocol buffers"
	@echo ""
	@echo "Running Nodes:"
	@echo "  run-validator               - Run a single validator node"
	@echo "  run-bootstrap               - Run a bootstrap node"
	@echo "  run-network                 - Run a network of 4 validators"
	@echo "  run-simulator               - Run the network simulator"
	@echo "  run-simulator-partition     - Run simulator with partitions"
	@echo ""
	@echo "Setup:"
	@echo "  deps                        - Install dependencies"
	@echo "  dev-setup                   - Setup development environment"
	@echo "  help                        - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make test-integration"
	@echo "  make test-integration-single TEST=TestSingleTicketValidation"
	@echo "  make test-integration-debug"
