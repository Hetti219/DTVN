.PHONY: all build clean test run-validator run-simulator docker-build docker-run help

# Variables
BINARY_NAME=validator
SIMULATOR_NAME=simulator
GO=go
DOCKER=docker
DOCKER_IMAGE=dtvn-validator

# Build all binaries
all: build

# Build validator and simulator
build:
	@echo "Building validator..."
	$(GO) build -o bin/$(BINARY_NAME) cmd/validator/main.go
	@echo "Building simulator..."
	$(GO) build -o bin/$(SIMULATOR_NAME) cmd/simulator/main.go
	@echo "Build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf data/
	rm -rf *.db
	@echo "Clean complete!"

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

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

# Run a network of 4 validator nodes
run-network:
	@echo "Starting validator network..."
	@mkdir -p data/node0 data/node1 data/node2 data/node3
	$(GO) run cmd/validator/main.go -id node0 -port 4001 -api-port 8081 -data-dir ./data/node0 -primary -total-nodes 4 & \
	sleep 2 && \
	$(GO) run cmd/validator/main.go -id node1 -port 4002 -api-port 8082 -data-dir ./data/node1 -total-nodes 4 -bootstrap "/ip4/127.0.0.1/tcp/4001" & \
	sleep 2 && \
	$(GO) run cmd/validator/main.go -id node2 -port 4003 -api-port 8083 -data-dir ./data/node2 -total-nodes 4 -bootstrap "/ip4/127.0.0.1/tcp/4001" & \
	sleep 2 && \
	$(GO) run cmd/validator/main.go -id node3 -port 4004 -api-port 8084 -data-dir ./data/node3 -total-nodes 4 -bootstrap "/ip4/127.0.0.1/tcp/4001"

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

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	$(DOCKER) build -t $(DOCKER_IMAGE):latest .

# Run Docker container
docker-run:
	@echo "Running Docker container..."
	$(DOCKER) run -d \
		-p 4001:4001 \
		-p 8080:8080 \
		-p 9090:9090 \
		--name validator-1 \
		$(DOCKER_IMAGE):latest

# Run Docker Compose (multi-node network)
docker-compose-up:
	@echo "Starting Docker Compose network..."
	docker-compose up -d

docker-compose-down:
	@echo "Stopping Docker Compose network..."
	docker-compose down

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
	@echo "  all                  - Build all binaries"
	@echo "  build                - Build validator and simulator"
	@echo "  clean                - Clean build artifacts"
	@echo "  test                 - Run tests"
	@echo "  test-coverage        - Run tests with coverage"
	@echo "  lint                 - Run linter"
	@echo "  fmt                  - Format code"
	@echo "  tidy                 - Tidy dependencies"
	@echo "  proto                - Generate protocol buffers"
	@echo "  run-validator        - Run a single validator node"
	@echo "  run-bootstrap        - Run a bootstrap node"
	@echo "  run-network          - Run a network of 4 validators"
	@echo "  run-simulator        - Run the network simulator"
	@echo "  run-simulator-partition - Run simulator with partitions"
	@echo "  docker-build         - Build Docker image"
	@echo "  docker-run           - Run Docker container"
	@echo "  docker-compose-up    - Start Docker Compose network"
	@echo "  docker-compose-down  - Stop Docker Compose network"
	@echo "  deps                 - Install dependencies"
	@echo "  dev-setup            - Setup development environment"
	@echo "  help                 - Show this help message"
