# Tiger CLI Development Guide

This guide provides information for developers who want to build, test, and contribute to Tiger CLI.

## Quick Start for Development

```bash
# Clone the repository
git clone https://github.com/timescale/tiger-cli.git
cd tiger-cli
git checkout <branch>

# Install the CLI
go install ./cmd/tiger

# (Optional) Set up the API endpoint
# For prod (default)
tiger config set console_url https://console.cloud.timescale.com
tiger config set gateway_url https://console.cloud.timescale.com/api
tiger config set api_url https://console.cloud.timescale.com/public/api/v1

# For dev
tiger config set console_url https://console.dev.timescale.com
tiger config set gateway_url https://console.dev.timescale.com/api
tiger config set api_url https://console.dev.timescale.com/public/api/v1

# For local development
tiger config set api_url http://localhost:8080/public/api/v1
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...
```

## Integration Tests

Integration tests execute real API calls against a Tiger environment to validate end-to-end functionality. These tests require valid credentials and will create/delete actual resources.

### Setup

1. Copy the sample environment file:
   ```bash
   cp .env.sample .env
   ```

2. Edit `.env` with your actual credentials:
   ```bash
   TIGER_PUBLIC_KEY_INTEGRATION=your-public-key-here
   TIGER_SECRET_KEY_INTEGRATION=your-secret-key-here
   TIGER_PROJECT_ID_INTEGRATION=your-project-id-here
   TIGER_API_URL=http://localhost:8080/public/api/v1  # or production URL
   ```

### Running Integration Tests

```bash
# Load environment variables and run all integration tests
export $(cat .env | xargs) && go test ./internal/tiger/cmd -v -run Integration

# Run specific integration test
export $(cat .env | xargs) && go test ./internal/tiger/cmd -v -run TestServiceLifecycleIntegration

# Integration tests will skip automatically if credentials are not set
go test ./internal/tiger/cmd -v -run Integration
```

### What Integration Tests Cover

- **Authentication lifecycle**: Login with credentials, verify authentication, logout
- **Service management**: Create, list, describe, and delete database services
- **Password management**: Update service passwords with keychain storage
- **Database connectivity**: Generate connection strings and execute psql commands
- **Output formats**: Validate JSON, YAML, and table output formats
- **Error handling**: Test authentication failures and resource cleanup

**Note**: Integration tests create and delete real services, which may incur costs. Use a development environment when possible.

## Project Structure

```
tiger-cli/
├── cmd/tiger/              # Main CLI entry point
├── internal/tiger/         # Internal packages
│   ├── api/                # Generated OpenAPI client (oapi-codegen)
│   │   └── mocks/          # Generated mocks for testing
│   ├── config/             # Configuration management
│   ├── logging/            # Structured logging utilities
│   ├── mcp/                # MCP server implementation
│   ├── password/           # Password storage utilities
│   ├── cmd/                # CLI commands (Cobra)
│   └── util/               # Shared utilities
├── docs/                   # Documentation
├── specs/                  # CLI specifications and API documentation
├── openapi.yaml            # OpenAPI 3.0 specification for Tiger API
└── tools.go                # Build-time dependencies
```

The `internal/` directory follows Go conventions to prevent external imports of internal packages.

## Architecture Overview

Tiger CLI is a Go-based command-line interface for managing Tiger resources. The architecture follows standard Go CLI patterns using Cobra and Viper.

### Key Components

- **Entry Point**: `cmd/tiger/main.go` - Simple main that delegates to cmd.Execute()
- **Command Structure**: `internal/tiger/cmd/` - Cobra-based command definitions
  - `root.go` - Root command with global flags and configuration initialization
  - `auth.go` - Authentication commands
  - `service.go` - Service management commands
  - `db.go` - Database operation commands
  - `mcp.go` - MCP server commands
  - `config.go` - Configuration management commands
  - `version.go` - Version command
- **Configuration**: `internal/tiger/config/config.go` - Centralized config with Viper integration
- **Logging**: `internal/tiger/logging/logging.go` - Structured logging with zap
- **API Client**: `internal/tiger/api/` - Generated OpenAPI client
- **MCP Server**: `internal/tiger/mcp/` - Model Context Protocol server implementation

### Configuration System

The CLI uses a layered configuration approach:
1. Default values in code
2. Configuration file at `~/.config/tiger/config.yaml`
3. Environment variables with `TIGER_` prefix
4. Command-line flags (highest precedence)

### Logging Architecture

Two-mode logging system using zap:
- **Production mode**: Minimal output, warn level and above, clean formatting
- **Debug mode**: Full development logging with colors and debug level

## Code Generation

```bash
# Generate OpenAPI client code and mocks from openapi.yaml
go generate ./internal/tiger/api

# Generates:
# - client.go: HTTP client implementation
# - types.go: Type definitions for API models
# - mocks/mock_client.go: Mock implementations for testing
```

## Dependencies

- **Cobra**: CLI framework and command structure
- **Viper**: Configuration management with multiple sources
- **Zap**: Structured logging
- **oapi-codegen**: OpenAPI client generation (build-time dependency)
- **gomock**: Mock generation for testing (build-time dependency)
- **go-sdk (MCP)**: Model Context Protocol SDK
- **Go 1.25+**: Required Go version

## Development Best Practices

1. **Always use go fmt** after making file changes and before committing
2. **Write tests** for new functionality
3. **Update documentation** when adding new features or commands
4. **Follow the existing code structure** and patterns
5. **Use the pure functional builder pattern** for new commands (see CLAUDE.md)
6. **Test with multiple output formats** (json, yaml, table)
7. **Validate configuration changes** don't break existing functionality

## Contributing Guidelines

1. **Fork the repository** on GitHub
2. **Create a feature branch** from `main`
3. **Make your changes** following the code style and patterns
4. **Add tests** for new functionality
5. **Run all tests** to ensure nothing breaks: `go test ./...`
6. **Run go fmt** to format your code: `go fmt ./...`
7. **Update documentation** if needed
8. **Submit a pull request** with a clear description of changes

### Pull Request Guidelines

- **Clear title**: Summarize the change in the PR title
- **Detailed description**: Explain what and why, not just how
- **Link issues**: Reference any related issues
- **Test evidence**: Show that tests pass
- **Breaking changes**: Clearly call out any breaking changes

## Release Process

To trigger the release pipeline, push a new semver tag (e.g. `v1.2.3`). This is
typically done by creating a new release in the GitHub UI, but it can also be
done at the command line:

```bash
VERSION=X.X.X && git tag -a v${VERSION} -m "${VERSION}" && git push origin v${VERSION} && git push
```

## Getting Help

- **GitHub Issues**: Report bugs or request features at https://github.com/timescale/tiger-cli/issues
- **Documentation**: Check the specs/ directory for detailed specifications

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](../LICENSE) file for details.
