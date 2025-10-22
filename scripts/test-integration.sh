#!/bin/bash

# Script to run integration tests
# Usage:
#   ./scripts/test-integration.sh                    # Run all integration tests
#   ./scripts/test-integration.sh CreateRole         # Run specific test pattern
#   ./scripts/test-integration.sh -v CreateRole      # Run with extra verbosity

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
TEST_PATTERN="${1:-Integration}"  # Default to all integration tests
VERBOSE=""
TIMEOUT="5m"

# Check for verbose flag
if [[ "$1" == "-v" ]] || [[ "$2" == "-v" ]]; then
    VERBOSE="-v"
    if [[ "$1" == "-v" ]]; then
        TEST_PATTERN="${2:-Integration}"
    fi
fi

echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}Tiger CLI Integration Test Runner${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"

# Check if .env exists
if [ ! -f .env ]; then
    echo -e "${RED}Error: .env file not found${NC}"
    echo ""
    echo "Please create a .env file with the required environment variables:"
    echo "  TIGER_PUBLIC_KEY_INTEGRATION=your-public-key"
    echo "  TIGER_SECRET_KEY_INTEGRATION=your-secret-key"
    echo "  TIGER_PROJECT_ID_INTEGRATION=your-project-id"
    echo "  TIGER_API_URL_INTEGRATION=https://api.tigerdata.cloud"
    echo ""
    echo "Optional:"
    echo "  TIGER_EXISTING_SERVICE_ID_INTEGRATION=existing-service-id"
    exit 1
fi

# Load environment variables from .env
echo -e "${GREEN}✓ Loading environment variables from .env${NC}"
export $(cat .env | grep -v '^#' | xargs)

# Build the binary first
echo -e "${GREEN}✓ Building tiger CLI...${NC}"
if go build -o bin/tiger ./cmd/tiger; then
    echo -e "${GREEN}  Build successful${NC}"
else
    echo -e "${RED}  Build failed${NC}"
    exit 1
fi

echo ""
echo -e "${BLUE}Running tests matching: ${TEST_PATTERN}${NC}"
echo -e "${BLUE}────────────────────────────────────────────────────────${NC}"

# Run the tests
if go test ./internal/tiger/cmd \
    $VERBOSE \
    -run "$TEST_PATTERN" \
    -timeout "$TIMEOUT"; then

    echo ""
    echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}✅ Tests completed successfully!${NC}"
    echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
    exit 0
else
    echo ""
    echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
    echo -e "${RED}❌ Tests failed${NC}"
    echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
    exit 1
fi