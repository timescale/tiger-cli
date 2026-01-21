# Tiger CLI

Tiger CLI is the command-line interface for Tiger Cloud. It provides commands for managing and querying database services, as well as an integrated Model Context Protocol (MCP) server for use with AI assistants.

## Installation

Multiple installation methods are provided. Choose your preferred method from the options below. If you aren't sure, use the first one!

### Install Script (macOS/Linux/WSL)

```bash
curl -fsSL https://cli.tigerdata.com | sh
```

### Install Script (Windows)

```powershell
irm https://cli.tigerdata.com/install.ps1 | iex
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
  - `status` - Show current authentication status and project ID
- `tiger service` - Service lifecycle management
  - `list` - List all services
  - `create` - Create a new service
  - `get` - Show detailed service information (aliases: `describe`, `show`)
  - `fork` - Fork an existing service
  - `start` - Start a stopped service
  - `stop` - Stop a running service
  - `resize` - Resize service CPU and memory allocation
  - `delete` - Delete a service
  - `update-password` - Update service master password
  - `logs` - View service logs
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
  - `list` - List available MCP tools, prompts, and resources
  - `get` - Get detailed information about a specific MCP capability (aliases: `describe`, `show`)
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
- `service_get` - Get detailed information about a specific service
- `service_create` - Create new database services with configurable resources
- `service_fork` - Fork an existing database service to create an independent copy
- `service_start` - Start a stopped database service
- `service_stop` - Stop a running database service
- `service_resize` - Resize a database service by changing CPU and memory allocation
- `service_update_password` - Update the master password for a service
- `service_logs` - View logs for a database service

**Database Operations:**
- `db_execute_query` - Execute SQL queries against a database service with support for parameterized queries, custom timeouts, and connection pooling

The MCP server automatically uses your CLI authentication and configuration, so no additional setup is required beyond `tiger auth login`.

#### Proxied Tools

In addition to the service management tools listed above, the Tiger MCP server also proxies tools from a remote documentation MCP server. This feature provides AI assistants with semantic search capabilities for PostgreSQL, TimescaleDB, and Tiger Cloud documentation, as well as prompts/guides for various Tiger Cloud features.

The proxied documentation server ([pg-aiguide](https://github.com/timescale/pg-aiguide)) currently provides the following tools:
- `view_skill` - Retrieve comprehensive guides for Postgres and TimescaleDB features and best practices
- `search_docs` - Search PostgreSQL and TimescaleDB documentation using natural language queries

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

- `analytics` - Enable/disable analytics (default: `true`)
- `color` - Enable/disable colored output (default: `true`)
- `debug` - Enable/disable debug logging (default: `false`)
- `docs_mcp` - Enable/disable docs MCP proxy (default: `true`)
- `output` - Output format: `json`, `yaml`, or `table` (default: `table`)
- `password_storage` - Password storage method: `keyring`, `pgpass`, or `none` (default: `keyring`)
- `service_id` - Default service ID
- `version_check_interval` - How often the CLI will check for new versions, 0 to disable (default: `24h`)

### Environment Variables

Environment variables override configuration file values. All variables use the `TIGER_` prefix:

- `TIGER_ANALYTICS` - Enable/disable analytics
- `TIGER_COLOR` - Enable/disable colored output
- `TIGER_CONFIG_DIR` - Path to configuration directory (default: `~/.config/tiger`)
- `TIGER_DEBUG` - Enable/disable debug logging
- `TIGER_DOCS_MCP` - Enable/disable docs MCP proxy
- `TIGER_OUTPUT` - Output format: `json`, `yaml`, or `table`
- `TIGER_PASSWORD_STORAGE` - Password storage method: `keyring`, `pgpass`, or `none`
- `TIGER_PUBLIC_KEY` - Public key to use for authentication (takes priority over stored credentials)
- `TIGER_SECRET_KEY` - Secret key to use for authentication (takes priority over stored credentials)
- `TIGER_SERVICE_ID` - Default service ID
- `TIGER_VERSION_CHECK_INTERVAL` - How often the CLI will check for new versions, 0 to disable

### Global Flags

These flags are available on all commands and take precedence over both environment variables and configuration file values:

- `--analytics` - Enable/disable analytics
- `--color` - Enable/disable colored output
- `--config-dir <path>` - Path to configuration directory (default: `~/.config/tiger`)
- `--debug` - Enable/disable debug logging
- `--password-storage <method>` - Password storage method: `keyring`, `pgpass`, or `none`
- `--service-id <id>` - Specify service ID
- `--skip-update-check` - Skip checking for updates on startup (default: `false`)
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
