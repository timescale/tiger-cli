# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building
```bash
# Build the main CLI binary
go build -o bin/tiger ./cmd/tiger

# Build from project root (creates bin/tiger)
go build -o bin/tiger ./cmd/tiger
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...
```

### Running Locally
```bash
# After building, run the CLI
./bin/tiger --help

# Or run directly with go
go run ./cmd/tiger --help
```

### Code Generation
```bash
# Generate OpenAPI client code and mocks from openapi.yaml
go generate ./internal/tiger/api

# This runs automatically as part of normal Go tooling when needed
# Generates:
# - client.go: HTTP client implementation
# - types.go: Type definitions for API models
# - mocks/mock_client.go: Mock implementations for testing
```

## Architecture Overview

Tiger CLI is a Go-based command-line interface for managing TigerData Cloud Platform resources. The architecture follows standard Go CLI patterns using Cobra and Viper.

### Key Components

- **Entry Point**: `cmd/tiger/main.go` - Simple main that delegates to cmd.Execute()
- **Command Structure**: `internal/tiger/cmd/` - Cobra-based command definitions
  - `root.go` - Root command with global flags and configuration initialization
  - `config.go` - Configuration management commands
  - `version.go` - Version command
- **Configuration**: `internal/tiger/config/config.go` - Centralized config with Viper integration
- **Logging**: `internal/tiger/logging/logging.go` - Structured logging with zap

### Configuration System

The CLI uses a layered configuration approach:
1. Default values in code
2. Configuration file at `~/.config/tiger/config.yaml`
3. Environment variables with `TIGER_` prefix
4. Command-line flags (highest precedence)

Key configuration values:
- `api_url`: TigerData API endpoint
- `project_id`: Default project ID
- `service_id`: Default service ID  
- `output`: Output format (json, yaml, table)
- `analytics`: Usage analytics toggle

### Logging Architecture

Two-mode logging system using zap:
- **Production mode**: Minimal output, warn level and above, clean formatting
- **Debug mode**: Full development logging with colors and debug level

Global flags available on all commands:
- `--debug`: Enable debug logging
- `--output/-o`: Set output format
- `--api-key`: Override API key
- `--project-id`: Override project ID
- `--service-id`: Override service ID
- `--analytics`: Toggle analytics

### Dependencies

- **Cobra**: CLI framework and command structure
- **Viper**: Configuration management with multiple sources
- **Zap**: Structured logging
- **oapi-codegen**: OpenAPI client generation (build-time dependency)
- **gomock**: Mock generation for testing (build-time dependency)
- **Go 1.23+**: Required Go version

## Project Structure

```
cmd/tiger/              # CLI entry point
internal/tiger/         # Internal packages
  cmd/                  # Cobra command definitions
  config/               # Configuration management
  logging/              # Structured logging utilities
  api/                  # Generated OpenAPI client (oapi-codegen)
    mocks/              # Generated mocks for testing
specs/                  # CLI specifications and API documentation
  spec.md               # Basic project specification
  spec_mcp.md           # MCP (Model Context Protocol) specification
bin/                    # Built binaries (created during build)
openapi.yaml            # OpenAPI 3.0 specification for TigerData API
tools.go                # Build-time dependencies
```

The `internal/` directory follows Go conventions to prevent external imports of internal packages.

## Cobra Usage Display Pattern

When implementing Cobra commands, use this pattern to control when usage information is displayed on errors:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    // 1. Do argument validation first - errors here show usage
    if len(args) < 1 {
        return fmt.Errorf("service ID is required")
    }
    
    // 2. Set SilenceUsage = true after argument validation
    cmd.SilenceUsage = true
    
    // 3. Proceed with business logic - errors here don't show usage
    if err := someAPICall(); err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }
    
    return nil
},
```

**Philosophy**: 
- Early argument/syntax errors → show usage (helps users learn command syntax)
- Operational errors after arguments are validated → don't show usage (avoids cluttering output with irrelevant usage info)

This provides fine-grained control over when usage is displayed, improving user experience by showing help when it's relevant and hiding it when it's not.

## Specifications

The project specifications are located in the `specs/` directory:
- `spec.md` - Basic project specification and CLI requirements
- `spec_mcp.md` - MCP (Model Context Protocol) specification for integration