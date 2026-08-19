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
tiger config set console_url https://console.cloud.tigerdata.com
tiger config set gateway_url https://console.cloud.tigerdata.com/api
tiger config set api_url https://console.cloud.tigerdata.com/public/api/v1

# For dev
tiger config set console_url https://console.dev.tigerdata.com
tiger config set gateway_url https://console.dev.tigerdata.com/api
tiger config set api_url https://console.dev.tigerdata.com/public/api/v1

# For development against local Tiger Cloud Console:
tiger config set console_url https://local.dev.tigerdata.com:8080

# For development against local REST API:
tiger config set api_url http://localhost:8080/public/api/v1
```

## Configuration Options

There are a handful of configuration options and environment variables that are specifically intended for use during development:

- `api_url` (`TIGER_API_URL`) - Tiger Cloud API endpoint (default: https://console.cloud.tigerdata.com/public/api/v1)
- `console_url` (`TIGER_CONSOLE_URL`) - Tiger Cloud Console URL (default: https://console.cloud.tigerdata.com)
- `gateway_url` (`TIGER_GATEWAY_URL`) - Tiger Cloud Gateway URL (default: https://console.cloud.tigerdata.com/api)
- `docs_mcp_url` (`TIGER_DOCS_MCP_URL`) - Docs MCP server URL (default: https://mcp.tigerdata.com/docs)

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
   TIGER_API_URL=http://localhost:8080/public/api/v1  # or production URL
   ```

### Running Integration Tests

```bash
# Load environment variables and run all integration tests
export $(cat .env | xargs) && go test ./internal/cmd -v -run Integration

# Run specific integration test
export $(cat .env | xargs) && go test ./internal/cmd -v -run TestServiceLifecycleIntegration

# Integration tests will skip automatically if credentials are not set
go test ./internal/cmd -v -run Integration
```

### What Integration Tests Cover

- **Authentication lifecycle**: Login with credentials, verify authentication, logout
- **Service management**: Create, list, get, and delete database services
- **Password management**: Update service passwords with keychain storage
- **Database connectivity**: Generate connection strings and execute psql commands
- **Output formats**: Validate JSON, YAML, and table output formats
- **Error handling**: Test authentication failures and resource cleanup
- **Self-upgrade**: With `TIGER_UPGRADE_INTEGRATION=1` set, `TestUpgradeLiveCDNIntegration` builds a dev binary and upgrades it in place against the live release CDN (runs in CI)

**Note**: Integration tests create and delete real services, which may incur costs. Use a development environment when possible.

## Project Structure

```
tiger-cli/
├── cmd/tiger/              # Main CLI entry point
├── internal/               # Internal packages
│   ├── api/                # Generated OpenAPI client (oapi-codegen)
│   │   └── mocks/          # Generated mocks for testing
│   ├── config/             # Configuration management
│   ├── mcp/                # MCP server implementation (one file per tool)
│   ├── common/             # Shared business logic used by CLI and MCP
│   ├── cmd/                # CLI commands (Cobra, one file per command)
│   └── util/               # Shared utilities
├── docs/                   # Documentation
├── openapi.yaml            # OpenAPI 3.0 specification for Tiger API
└── tools.go                # Build-time dependencies
```

The `internal/` directory follows Go conventions to prevent external imports of internal packages.

## Architecture Overview

Tiger CLI is a Go-based command-line interface for managing Tiger resources. The architecture follows standard Go CLI patterns using Cobra and Viper.

### Key Components

- **Entry Point**: `cmd/tiger/main.go` - Simple main that delegates to cmd.Execute()
- **Command Structure**: `internal/cmd/` - Cobra-based command definitions for all
  CLI commands (auth, service, db, config, mcp, version, upgrade). Each command
  lives in its own file, named to match the command in snake_case
  (`tiger service create` → `service_create.go`). `root.go` holds the root
  command, global flags, and `wrapCommands`, which gives every command the same
  per-invocation lifecycle: load config + API client once into a `common.App`,
  apply color settings, check for a newer release, and track analytics.
- **App**: `internal/common/app.go` - per-invocation config and API client, built
  once by `wrapCommands` (or per request by the MCP analytics middleware) and read
  by commands, MCP tool handlers, and completion functions
- **Configuration**: `internal/config/config.go` - `Config` struct plus load/write
  helpers. `config.Load(flags)` resolves values through a per-call viper
  instance (flag > env > file > default); there is no global config state
- **API Client**: `internal/api/` - Generated OpenAPI client
- **MCP Server**: `internal/mcp/` - Model Context Protocol server
  implementation. Each MCP tool lives in its own file, named to match the tool
  (`service_create` → `service_create.go`).

### Configuration System

The CLI uses a layered configuration approach (listed from lowest to highest precedence):
1. Default values in code
2. Configuration file at `~/.config/tiger/config.yaml`
3. Environment variables with `TIGER_` prefix
4. Command-line flags (highest precedence)

### Logging Architecture

Only the MCP server logs. `tiger mcp start` builds a `*slog.Logger` with
`newLogger(cmd.ErrOrStderr())` (`internal/cmd/logger_helper.go`), which points
the standard `log` package at that writer and returns `slog.Default()`. That
logger is passed to `mcp.NewServer`, which uses it for its own output and hands
it to the MCP SDK (`mcp.ServerOptions.Logger`); a nil logger is discarded.
Everything else writes to stdout/stderr directly rather than logging.

## Code Generation

```bash
# Generate OpenAPI client code and mocks from openapi.yaml
go generate ./internal/api

# Generates:
# - client.go: HTTP client implementation
# - types.go: Type definitions for API models
# - mocks/mock_client.go: Mock implementations for testing
```

Generation is configured by `internal/api/types.yaml` and
`internal/api/client.yaml`. Both normalize generated names to Go's initialism
conventions (`ServiceID`, `CPUMillis`) and prefix enum constants with their type
name (`api.DeployStatusREADY`).

## Development Best Practices

1. **Always use go fmt** after making file changes and before committing
2. **Write tests** for new functionality
3. **Update documentation** when adding new features or commands
4. **Follow the existing code structure** and patterns
5. **Use the pure functional builder pattern** for new commands (see CLAUDE.md)
6. **Test with multiple output formats** (json, yaml, table)
7. **Validate configuration changes** don't break existing functionality
8. **Use CLAUDE.md** other guidelines are listed there and you can use it with any AI-enabled coding editor.

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
- **Documentation**: See [README.md](../README.md) for user-facing docs

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](../LICENSE) file for details.
