#!/bin/bash

# Integration Test Runner Script for DTVN
# This script builds the validator binary and runs all integration tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/bin"
DATA_DIR="${PROJECT_ROOT}/test-data/integration"
LOGS_DIR="${PROJECT_ROOT}/test-data/logs"
VALIDATOR_BINARY="${BIN_DIR}/validator"

# Test configuration
PARALLEL=false
VERBOSE=false
CLEANUP=true
TEST_PATTERN=""
TIMEOUT="30m"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -p|--parallel)
      PARALLEL=true
      shift
      ;;
    -v|--verbose)
      VERBOSE=true
      shift
      ;;
    --no-cleanup)
      CLEANUP=false
      shift
      ;;
    --pattern)
      TEST_PATTERN="$2"
      shift 2
      ;;
    --timeout)
      TIMEOUT="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  -p, --parallel      Run tests in parallel"
      echo "  -v, --verbose       Enable verbose output"
      echo "  --no-cleanup        Don't cleanup test data after run"
      echo "  --pattern PATTERN   Run only tests matching pattern"
      echo "  --timeout DURATION  Test timeout (default: 30m)"
      echo "  -h, --help          Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Print banner
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}     DTVN Integration Test Suite${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Step 1: Cleanup old test data
if [ "$CLEANUP" = true ]; then
  echo -e "${YELLOW}[1/5]${NC} Cleaning up old test data..."
  rm -rf "${DATA_DIR}" "${LOGS_DIR}" 2>/dev/null || true
  echo -e "${GREEN}✓${NC} Cleanup complete"
else
  echo -e "${YELLOW}[1/5]${NC} Skipping cleanup (--no-cleanup flag)"
fi

# Step 2: Create test directories
echo -e "${YELLOW}[2/5]${NC} Creating test directories..."
mkdir -p "${DATA_DIR}"
mkdir -p "${LOGS_DIR}"
mkdir -p "${BIN_DIR}"
echo -e "${GREEN}✓${NC} Directories created"

# Step 3: Build validator binary
echo -e "${YELLOW}[3/5]${NC} Building validator binary..."
cd "${PROJECT_ROOT}"

if [ "$VERBOSE" = true ]; then
  go build -v -o "${VALIDATOR_BINARY}" ./cmd/validator
else
  go build -o "${VALIDATOR_BINARY}" ./cmd/validator 2>&1 | grep -v "^#" || true
fi

if [ ! -f "${VALIDATOR_BINARY}" ]; then
  echo -e "${RED}✗${NC} Failed to build validator binary"
  exit 1
fi

echo -e "${GREEN}✓${NC} Validator binary built: ${VALIDATOR_BINARY}"

# Step 4: Run integration tests
echo -e "${YELLOW}[4/5]${NC} Running integration tests..."
echo ""

# Build test command
TEST_CMD="go test"
TEST_ARGS="-timeout ${TIMEOUT} -v"

if [ "$PARALLEL" = true ]; then
  TEST_ARGS="${TEST_ARGS} -p 4"
fi

if [ -n "$TEST_PATTERN" ]; then
  TEST_ARGS="${TEST_ARGS} -run ${TEST_PATTERN}"
fi

# Set test directory (use relative path to avoid issues with spaces)
TEST_DIR="./test/integration/scenarios"

# Run tests
echo -e "${BLUE}Running:${NC} ${TEST_CMD} ${TEST_ARGS} ${TEST_DIR}"
echo ""

cd "${PROJECT_ROOT}"

if [ "$VERBOSE" = true ]; then
  ${TEST_CMD} ${TEST_ARGS} "${TEST_DIR}"
  TEST_EXIT_CODE=$?
else
  ${TEST_CMD} ${TEST_ARGS} "${TEST_DIR}" 2>&1 | while IFS= read -r line; do
    if echo "$line" | grep -q -- "PASS:"; then
      echo -e "${GREEN}${line}${NC}"
    elif echo "$line" | grep -q -- "FAIL:"; then
      echo -e "${RED}${line}${NC}"
    elif echo "$line" | grep -q -- "ok"; then
      echo -e "${GREEN}${line}${NC}"
    elif echo "$line" | grep -q -- "FAIL"; then
      echo -e "${RED}${line}${NC}"
    elif echo "$line" | grep -q -- "^---"; then
      echo -e "${BLUE}${line}${NC}"
    elif echo "$line" | grep -q -- "^==="; then
      echo -e "${YELLOW}${line}${NC}"
    else
      echo "$line"
    fi
  done
  TEST_EXIT_CODE=${PIPESTATUS[0]}
fi

echo ""

# Step 5: Report results
echo -e "${YELLOW}[5/5]${NC} Test Summary"
echo "═══════════════════════════════════════════════════════════"

if [ $TEST_EXIT_CODE -eq 0 ]; then
  echo -e "${GREEN}✓ All integration tests passed!${NC}"
  EXIT_CODE=0
else
  echo -e "${RED}✗ Some integration tests failed${NC}"
  EXIT_CODE=1
fi

echo ""
echo "Test artifacts:"
echo "  - Data directory: ${DATA_DIR}"
echo "  - Logs directory: ${LOGS_DIR}"
echo "  - Validator binary: ${VALIDATOR_BINARY}"

# Cleanup if requested
if [ "$CLEANUP" = true ] && [ $EXIT_CODE -eq 0 ]; then
  echo ""
  echo "Cleaning up test artifacts..."
  rm -rf "${DATA_DIR}" "${LOGS_DIR}"
  echo -e "${GREEN}✓${NC} Cleanup complete"
fi

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"

exit $EXIT_CODE
