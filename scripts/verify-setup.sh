#!/bin/bash

# Distributed Ticket Validation Network - Setup Verification Script

echo "=== DTVN Setup Verification ==="
echo ""

# Check Go installation
echo "Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version)
    echo "✓ Go is installed: $GO_VERSION"
else
    echo "✗ Go is not installed"
    exit 1
fi

# Check Go module
echo ""
echo "Checking Go module..."
if [ -f "go.mod" ]; then
    echo "✓ go.mod found"
    MODULE=$(grep "^module" go.mod | awk '{print $2}')
    echo "  Module: $MODULE"
else
    echo "✗ go.mod not found"
    exit 1
fi

# Check dependencies
echo ""
echo "Checking dependencies..."
go list -m all > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "✓ Dependencies resolved"
    DEP_COUNT=$(go list -m all | wc -l)
    echo "  Total dependencies: $DEP_COUNT"
else
    echo "✗ Failed to resolve dependencies"
    exit 1
fi

# Check project structure
echo ""
echo "Checking project structure..."
REQUIRED_DIRS=("cmd/validator" "cmd/simulator" "pkg/network" "pkg/gossip" "pkg/consensus" "pkg/state" "pkg/storage" "pkg/api" "internal/crypto" "internal/types" "internal/metrics" "proto" "config")

for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        echo "✓ $dir"
    else
        echo "✗ $dir (missing)"
    fi
done

# Try to build
echo ""
echo "Attempting to build..."
mkdir -p bin

echo "  Building validator..."
go build -o bin/validator cmd/validator/main.go 2>&1 | head -10
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo "✓ Validator built successfully"
else
    echo "⚠ Validator build has issues (check output above)"
fi

echo "  Building simulator..."
go build -o bin/simulator cmd/simulator/main.go 2>&1 | head -10
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo "✓ Simulator built successfully"
else
    echo "⚠ Simulator build has issues (check output above)"
fi

echo ""
echo "=== Verification Complete ==="
echo ""
echo "Next steps:"
echo "1. Run 'make build' to build all binaries"
echo "2. Run 'make run-validator' to start a single validator"
echo "3. Run 'make run-network' to start a full network"
echo "4. Run 'make run-simulator' to run the network simulator"
echo "5. Check README.md for detailed instructions"
