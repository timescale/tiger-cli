# Tiger CLI

Tiger CLI is the command-line interface for managing databases on Tiger Cloud.

## Installation

### Install Script

```bash
curl -fsSL https://tiger-cli-releases.s3.amazonaws.com/install/install.sh | sh
```

### Homebrew (macOS/Linux)

```bash
brew install --cask timescale/tap/tiger-cli
```

### Debian/Ubuntu

```bash
# Add repository
curl -s https://packagecloud.io/install/repositories/timescale/tiger-cli/script.deb.sh | sudo os=any dist=any bash

# Install tiger-cli
sudo apt-get install tiger-cli
```

For manual repository installation instructions, see [here](https://packagecloud.io/timescale/tiger-cli/install#manual-deb).

### Red Hat/Fedora

```bash
# Add repository
curl -s https://packagecloud.io/install/repositories/timescale/tiger-cli/script.rpm.sh | sudo os=rpm_any dist=rpm_any bash

# Install tiger-cli
sudo yum install tiger-cli
```

For manual repository installation instructions, see [here](https://packagecloud.io/timescale/tiger-cli/install#manual-rpm).

### Go Install

```bash
go install github.com/timescale/tiger-cli/cmd/tiger@latest
```

## Quick Start

After installing Tiger CLI, authenticate with your Tiger Cloud account:

```bash
# Login to your Tiger account
tiger auth login

# View available commands
tiger --help

# List your database services
tiger service list

# Create a new database service
tiger service create --name my-database

# Get connection string
tiger db connection-string

# Connect to your database
tiger db connect

# Install the MCP server
tiger mcp install
```

## Usage

Tiger CLI provides the following commands:

- `tiger auth` - Authentication management
  - `login` - Log in to your Tiger account
  - `logout` - Log out from your Tiger account
  - `whoami` - Show current authentication status
- `tiger service` - Service lifecycle management
  - `list` - List all services
  - `create` - Create a new service
  - `describe` - Show detailed service information
  - `fork` - Fork an existing service
  - `delete` - Delete a service
  - `update-password` - Update service master password
- `tiger db` - Database operations
  - `connect` - Connect to a database with psql
  - `connection-string` - Get connection string for a service
  - `test-connection` - Test database connectivity
- `tiger config` - Configuration management
  - `show` - Show current configuration
  - `set` - Set configuration value
  - `unset` - Remove configuration value
  - `reset` - Reset configuration to defaults
- `tiger mcp` - MCP server setup and management
  - `install` - Install and configure MCP server for an AI assistant
  - `start` - Start the MCP server
- `tiger version` - Show version information

Use `tiger <command> --help` for detailed information about each command.

## MCP Server

Tiger CLI includes a Model Context Protocol (MCP) server that enables AI assistants like Claude Code to interact with your Tiger Cloud infrastructure. The MCP server provides programmatic access to database services and operations.

### Installation

Configure the MCP server for your AI assistant:

```bash
# Interactive installation (prompts for client selection)
tiger mcp install

# Or specify your client directly
tiger mcp install claude-code    # Claude Code
tiger mcp install codex          # Codex
tiger mcp install cursor         # Cursor IDE
tiger mcp install gemini         # Gemini CLI
tiger mcp install vscode         # VS Code
tiger mcp install windsurf       # Windsurf
```

After installation, restart your AI assistant to activate the Tiger MCP server.

#### Manual Installation

If your MCP client is not supported by `tiger mcp install`, follow the client's
instructions for installing MCP servers. Use `tiger mcp start` as the command to
start the MCP server. For example, many clients use a JSON file like the
following:


```json
{
  "mcpServers": {
    "tiger": {
      "command": "tiger",
      "args": [
        "mcp",
        "start"
      ]
    }
  }
}
```

#### Streamable HTTP Protocol

The above instructions install the MCP server using the stdio transport. If you
need to use the Streamable HTTP transport instead, you can start the server with
`tiger mcp start http --port 8080` and install it into your client using
`http://localhost:8080` as the URL.

### Available MCP Tools

The MCP server exposes the following tools to AI assistants:

**Service Management:**
- `service_list` - List all database services in your project
- `service_show` - Show detailed information about a specific service
- `service_create` - Create new database services with configurable resources
- `service_update_password` - Update the master password for a service

The MCP server automatically uses your CLI authentication and configuration, so no additional setup is required beyond `tiger auth login`.

#### Proxied Tools

In addition to the service management tools listed above, the Tiger MCP server also proxies tools from a remote documentation MCP server. This feature provides AI assistants with semantic search capabilities for PostgreSQL, TimescaleDB, and Tiger Cloud documentation, as well as prompts/guides for various Tiger Cloud features.

The proxied documentation server ([tiger-docs-mcp-server](https://github.com/timescale/tiger-docs-mcp-server)) currently provides the following tools:
- `get_guide` - Retrieve comprehensive guides for TimescaleDB features and best practices
- `semantic_search_postgres_docs` - Search PostgreSQL documentation using natural language queries
- `semantic_search_tiger_docs` - Search Tiger Cloud and TimescaleDB documentation using natural language queries

This proxy connection is enabled by default and requires no additional configuration.

To disable the documentation proxy:

```bash
tiger config set docs_mcp false
```

## Configuration

The CLI stores configuration in `~/.config/tiger/config.yaml` by default, and supports hierarchical configuration through environment variables and command-line flags.

```bash
# Show current configuration
tiger config show

# Set configuration values
tiger config set output json

# Remove configuration value
tiger config unset output

# Reset to defaults
tiger config reset
```

### Configuration Options

All configuration options can be set via `tiger config set <key> <value>`:

- `docs_mcp` - Enable/disable docs MCP proxy (default: `true`)
- `project_id` - Default project ID (set via `tiger auth login`)
- `service_id` - Default service ID
- `output` - Output format: `json`, `yaml`, or `table` (default: `table`)
- `analytics` - Enable/disable analytics (default: `true`)
- `password_storage` - Password storage method: `keyring`, `pgpass`, or `none` (default: `keyring`)
- `debug` - Enable/disable debug logging (default: `false`)

### Environment Variables

Environment variables override configuration file values. All variables use the `TIGER_` prefix:

- `TIGER_CONFIG_DIR` - Path to configuration directory (default: `~/.config/tiger`)
- `TIGER_DOCS_MCP` - Enable/disable docs MCP proxy
- `TIGER_PROJECT_ID` - Default project ID
- `TIGER_SERVICE_ID` - Default service ID
- `TIGER_OUTPUT` - Output format: `json`, `yaml`, or `table`
- `TIGER_ANALYTICS` - Enable/disable analytics
- `TIGER_PASSWORD_STORAGE` - Password storage method: `keyring`, `pgpass`, or `none`
- `TIGER_DEBUG` - Enable/disable debug logging

### Global Flags

These flags are available on all commands and take precedence over both environment variables and configuration file values:

- `--config-dir <path>` - Path to configuration directory (default: `~/.config/tiger`)
- `--project-id <id>` - Specify project ID
- `--service-id <id>` - Specify service ID
- `-o, --output <format>` - Output format: `json`, `yaml`, `env`, or `table`
- `--analytics` - Enable/disable analytics
- `--password-storage <method>` - Password storage method: `keyring`, `pgpass`, or `none`
- `--debug` - Enable/disable debug logging
- `-h, --help` - Show help information

## Contributing

We welcome contributions! Here's how to get started:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass (`go test ./...`)
6. Submit a pull request

For detailed development information, see [docs/development.md](docs/development.md).

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
