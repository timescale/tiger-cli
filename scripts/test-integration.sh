#!/bin/bash

# Script to run integration tests
# Usage:
#   ./scripts/test-integration.sh                    # Run all integration tests
#   ./scripts/test-integration.sh -v                 # Run with verbose output
#   ./scripts/test-integration.sh -run CreateRole    # Run specific test pattern
#   ./scripts/test-integration.sh -timeout 10m       # Set custom timeout
#
# Loads environment variables from .env file and passes all arguments to go test.
# If .env doesn't exist, shows helpful error message.

if [ ! -f .env ]; then
    echo "Error: .env file not found"
    echo ""
    echo "Please create a .env file with the required environment variables:"
    echo "  TIGER_PUBLIC_KEY_INTEGRATION=your-public-key"
    echo "  TIGER_SECRET_KEY_INTEGRATION=your-secret-key"
    echo "  TIGER_API_URL_INTEGRATION=https://console.cloud.timescale.com/public/api/v1"
    echo ""
    echo "Optional:"
    echo "  TIGER_EXISTING_SERVICE_ID_INTEGRATION=existing-service-id"
    exit 1
fi

# Build the binary first (needed for integration tests that exec the CLI)
go build -o bin/tiger ./cmd/tiger || exit 1

# Run tests with env vars from .env file, defaulting to Integration pattern
env $(cat .env | grep -v '^#' | xargs) go test ./... -run Integration "$@"
