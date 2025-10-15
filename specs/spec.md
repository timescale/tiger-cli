# Tiger CLI Specification

## Overview

The `tiger` CLI is a command-line interface for managing TigerData Cloud Platform resources. Built as a single Go binary, it provides comprehensive tools for managing database services, VPCs, replicas, and related infrastructure components.

## Installation

```bash
# Homebrew (macOS/Linux)
brew install tigerdata/tap/tiger

# Go install
go install github.com/tigerdata/cli/tiger@latest

# Direct binary download
curl -L https://github.com/tigerdata/cli/releases/latest/download/tiger-$(uname -s)-$(uname -m) -o tiger
chmod +x tiger
mv tiger /usr/local/bin/
```

## Global Configuration

### Environment Variables

- `TIGER_PUBLIC_KEY`: TigerData public key for authentication
- `TIGER_SECRET_KEY`: TigerData secret key for authentication
- `TIGER_PROJECT_ID`: Default project ID to use
- `TIGER_API_URL`: Base URL for TigerData API (default: https://api.tigerdata.com/public/v1)
- `TIGER_SERVICE_ID`: Default service ID to use
- `TIGER_CONFIG_DIR`: Configuration directory (default: ~/.config/tiger)
- `TIGER_OUTPUT`: Default output format (json, yaml, table)
- `TIGER_ANALYTICS`: Enable/disable usage analytics (true/false)
- `TIGER_NEW_PASSWORD`: Password to use for save-password command (overridden by --password flag)

### Configuration File

Location: `~/.config/tiger/config.yaml`

```yaml
api_url: https://api.tigerdata.com/public/v1
project_id: your-default-project-id
service_id: your-default-service-id
output: table
analytics: true
```

### Global Options

- `--config-dir`: Path to configuration directory
- `--project-id`: Specify project ID
- `--service-id`: Override default service ID (can also be specified positionally for single-service commands)
- `--analytics`: Toggle analytics collection
- `--password-storage`: Password storage method (keyring, pgpass, none) (default: keyring)
- `-v, --version`: Show CLI version
- `-h, --help`: Show help information
- `--debug`: Enable debug logging

## Command Structure

```
tiger [global-options] <command> [subcommand] [options] [arguments]
```

## v0 CLI Priority

For the initial v0 release, implement these essential commands first:

**Authentication:**
- `tiger auth login` - Token-based authentication
- `tiger auth logout` - Remove stored credentials
- `tiger auth status` - Show current user

**Core Service Management:**
- `tiger service list` - List all services
- `tiger service get` - Show service details (aliases: `describe`, `show`)
- `tiger service create` - Create new services
- `tiger service delete` - Delete services with confirmation
- `tiger service update-password` - Update service master password

**Database Operations:**
- `tiger db connect` / `tiger db psql` - Connect to databases
- `tiger db connection-string` - Get connection strings
- `tiger db test-connection` - Test connectivity

**Configuration:**
- `tiger config show` - Show current configuration
- `tiger config set` - Set configuration values

**Future v1+ Commands:**
- Service lifecycle (start/stop/restart) - pending API endpoints
- HA management commands
- Read replica management
- VPC management and peering
- Advanced service operations (resize, pooler, VPC attach/detach)
- Password management (save/remove from .pgpass)
- MCP server management

## Commands

### Authentication

#### `tiger auth`
Manage authentication and credentials (token-based only).

**Subcommands:**
- `login`: Authenticate with API token
- `logout`: Remove stored credentials
- `status`: Show current user information

**Examples:**
```bash
# Login with all credentials via flags
tiger auth login --public-key YOUR_PUBLIC_KEY --secret-key YOUR_SECRET_KEY --project-id proj-123

# Login with project ID flag (will prompt for keys if not in environment)
tiger auth login --project-id proj-123

# Login with credentials from environment variables
export TIGER_PUBLIC_KEY="your-public-key"
export TIGER_SECRET_KEY="your-secret-key"  
export TIGER_PROJECT_ID="proj-123"
tiger auth login

# Interactive login (will prompt for any missing credentials)
tiger auth login

# Show current user
tiger auth status

# Logout
tiger auth logout
```

**Authentication Methods:**
1. `--public-key` and `--secret-key` flags: Provide public and secret keys directly
2. `TIGER_PUBLIC_KEY` and `TIGER_SECRET_KEY` environment variables
3. `TIGER_PROJECT_ID` environment variable for project ID
4. Interactive prompt for any missing credentials (requires TTY)

**Login Process:**
When using `tiger auth login`, the CLI will:
1. Prompt for any missing credentials (public key, secret key, project ID) if not provided via flags or environment variables
2. Combine the public and secret keys into format `<public key>:<secret key>` for internal storage
3. Validate the combined API key by making a test API call
4. Store the combined API key securely using system keyring or file fallback
5. Store the project ID in `~/.config/tiger/config.yaml` as the default project

**Project ID Requirement:**
Project ID is required during login. The CLI will:
- Use `--project-id` flag if provided
- Use `TIGER_PROJECT_ID` environment variable if flag not provided  
- Prompt interactively for project ID if neither flag nor environment variable is set
- Fail with an error in non-interactive environments (no TTY) if project ID is missing

**API Key Storage:**
The combined API key (`<public key>:<secret key>`) is stored securely using:
1. **System keyring** (preferred): Uses [go-keyring](https://github.com/zalando/go-keyring) for secure storage in system credential managers (macOS Keychain, Windows Credential Manager, Linux Secret Service)  
2. **File fallback**: If keyring is unavailable, stores in `~/.config/tiger/api-key` with restricted file permissions (600)

**Options:**
- `--public-key`: Public key for authentication
- `--secret-key`: Secret key for authentication
- `--project-id`: Project ID to set as default in configuration

### Service Management

#### `tiger service`
Manage database services.

**Aliases:** `services`, `svc`

**Subcommands:**
- `list`: List all services
- `get`: Show service details (aliases: `describe`, `show`)
- `create`: Create a new service
- `delete`: Delete a service
- `start`: Start a service
- `stop`: Stop a service
- `restart`: Restart a service
- `resize`: Resize service resources
- `rename`: Rename a service
- `attach-vpc`: Attach service to a VPC
- `detach-vpc`: Detach service from VPC
- `enable-pooler`: Enable connection pooling
- `disable-pooler`: Disable connection pooling
- `update-password`: Update service master password
- `set-default`: Set default service

**Commands with Wait/Timeout Behavior:**
The following commands support `--wait`, `--no-wait`, and `--wait-timeout` options:
- `create`: Wait for service to be ready
- `delete`: Wait for service to be fully deleted
- `start`: Wait for service to be running
- `stop`: Wait for service to be stopped
- `restart`: Wait for service to restart completely
- `resize`: Wait for resize operation to complete
- `attach-vpc`: Wait for VPC attachment to complete
- `detach-vpc`: Wait for VPC detachment to complete
- `enable-pooler`: Wait for pooler to be enabled
- `disable-pooler`: Wait for pooler to be disabled

**Examples:**
```bash
# List services (all forms work identically)
tiger service list
tiger services list
tiger svc list

# Show service details (all forms work)
tiger service get svc-12345
tiger service describe svc-12345  # alias
tiger service show svc-12345      # alias
tiger svc get svc-12345

# Create a free tier service
tiger service create \
  --name "free-db" \
  --cpu shared \
  --memory shared

# Create a TimescaleDB service
tiger service create \
  --name "production-db" \
  --addons time-series \
  --cpu 500m \
  --memory 2GB

# Create a PostgreSQL service (waits for ready by default)
tiger service create \
  --name "postgres-db" \
  --addons none \
  --cpu 500m \
  --memory 2GB

# Create service without waiting
tiger service create \
  --name "quick-service" \
  --no-wait

# Create service with custom timeout
tiger service create \
  --name "patient-service" \
  --wait-timeout 60m

# Delete service (with confirmation prompt - will prompt to type service ID)
tiger service delete svc-12345

# Delete service without confirmation prompt (for automation)
tiger service delete svc-12345 --confirm

# Delete service without waiting for completion  
tiger service delete svc-12345 --confirm --no-wait

# Delete service with custom wait timeout
tiger service delete svc-12345 --confirm --wait-timeout 15m

# Resize service
tiger service resize svc-12345 --cpu 4 --memory 16GB

# Resize service without waiting
tiger service resize svc-12345 --cpu 4 --memory 16GB --no-wait

# Resize service with custom timeout
tiger service resize svc-12345 --cpu 4 --memory 16GB --wait-timeout 15m

# Start/stop service
tiger service start svc-12345
tiger service stop svc-12345

# Stop service without waiting
tiger service stop svc-12345 --no-wait

# Stop service with custom timeout
tiger service stop svc-12345 --wait-timeout 10m

# Attach/detach VPC
tiger service attach-vpc svc-12345 --vpc-id vpc-67890
tiger service detach-vpc svc-12345 --vpc-id vpc-67890

# Enable/disable connection pooling
tiger service enable-pooler svc-12345
tiger service disable-pooler svc-12345

# Update service password
tiger service update-password svc-12345 --password new-secure-password

# Set default service
tiger service set-default svc-12345
```

**Options:**
- `--name`: Service name (auto-generated if not provided)
- `--addons`: Addons to enable (time-series, ai, or 'none' for PostgreSQL-only)
- `--region`: Region code
- `--cpu`: CPU allocation - must be from allowed configurations (see below), or 'shared' for free tier
- `--memory`: Memory allocation - must be from allowed configurations (see below), or 'shared' for free tier
- `--replicas`: Number of high-availability replicas (default: 0)
- `--vpc-id`: VPC ID for attach/detach operations
- `--set-default`: Set this service as the default service (default: true)
- `--no-set-default`: Don't set this service as the default service
- `--wait`: Wait for operation to complete (default: true for commands listed above)
- `--no-wait`: Don't wait for operation to complete, return immediately
- `--wait-timeout`: Timeout for waiting (accepts any duration format from Go's time.ParseDuration: "30m", "1h30m", "90s", etc.) (default: 30m)
- `--password`: New password (for update-password command)
- `--confirm`: Skip confirmation prompt for destructive operations (AI agents must confirm with user first)

**Default Behavior:**
- **Password Management**: Password storage is controlled by the global `--password-storage` flag (keyring by default). When `--password-storage=keyring`, passwords are stored in the system keyring. When `--password-storage=pgpass`, passwords are saved to `~/.pgpass` for automatic authentication. When `--password-storage=none`, passwords are not saved automatically and must be managed manually.
- **Default Service**: By default, newly created services will be set as the default service in your configuration. Use `--no-set-default` to disable this behavior and keep your current default service unchanged.
- **Wait for Completion**: By default, asynchronous service commands (see list above) will wait for the operation to complete before returning, displaying status updates every 10 seconds. Use `--no-wait` to return immediately after the request is accepted, or `--wait-timeout` to specify a custom timeout period. If the wait timeout is exceeded, the command will exit with code 2, but the operation will continue running on the server.
- **Delete Safety**: The `delete` command requires an explicit service ID (no default fallback) and prompts users to type the exact service ID to confirm deletion. The `--confirm` flag skips this prompt for automation use cases. **Important for AI agents**: Always confirm with users before performing any delete operation, whether using interactive prompts or the `--confirm` flag.

**Allowed CPU/Memory Configurations:**
Service creation and resizing support the following CPU and memory combinations. You can specify both CPU and memory together, or specify only one (the other will be automatically set to the corresponding value):

| CPU | Memory |
|-----|---------|
| shared | shared |
| 0.5 | 2GB |
| 1 | 4GB |
| 2 | 8GB |
| 4 | 16GB |
| 8 | 32GB |
| 16 | 64GB |
| 32 | 128GB |

CPU must be specified as millicores (e.g., "500", "1000", "2000"), or "shared" for free tier services.
Memory must be specified as GBs (e.g., "2", "4", "8", "16", "32", "64", "128") or "shared" for free tier services.

**Examples:**
```bash
# Create free tier service
tiger service create --name "free-db" --cpu shared --memory shared

# Specify both CPU and memory
tiger service create --name "my-service" --cpu 2 --memory 8

# Specify only CPU (memory will be automatically set to 8)
tiger service create --name "my-service" --cpu 2

# Specify only memory (CPU will be automatically set to 2)
tiger service create --name "my-service" --memory 8

# Resize with only CPU
tiger service resize svc-12345 --cpu 4

# Resize with only memory
tiger service resize svc-12345 --memory 16
```

**Note:** A future command like `tiger service list-types` or `tiger service list-configurations` should be added to programmatically discover available service types, CPU/memory configurations, and regions without requiring users to reference documentation.

### Database Operations

#### `tiger db`
Database-specific operations and management.

**Subcommands:**
- `connect`: Connect to a database
- `psql`: Connect to a database (alias for connect)
- `connection-string`: Get connection string for a service
- `test-connection`: Test database connectivity
- `save-password`: Save password according to --password-storage setting
- `remove-password`: Remove password from configured storage location

**Examples:**
```bash
# Connect to database
tiger db connect svc-12345
# or use psql alias
tiger db psql svc-12345

# Connect with custom role/username
tiger db connect svc-12345 --role readonly
tiger db psql svc-12345 --role readonly

# Connect using connection pooler (if available)
tiger db connect svc-12345 --pooled
tiger db psql svc-12345 --pooled

# Pass additional flags to psql (use -- to separate)
tiger db connect svc-12345 -- --single-transaction --quiet
tiger db psql svc-12345 -- -c "SELECT version();" --no-psqlrc

# Combine tiger flags with psql flags
tiger db connect svc-12345 --pooled --role readonly -- --no-psqlrc -v ON_ERROR_STOP=1

# Get connection string
tiger db connection-string svc-12345
# Get pooled connection string
tiger db connection-string svc-12345 --pooled

# Test database connectivity
tiger db test-connection svc-12345

# Test with custom timeout
tiger db test-connection svc-12345 --timeout 10s

# Save password with explicit value (highest precedence)
tiger db save-password svc-12345 --password your-password
tiger db save-password svc-12345 --password your-password --role readonly

# Interactive password prompt (when --password flag provided with no value)
tiger db save-password svc-12345 --password

# Using environment variable (only when --password flag not provided)
export TIGER_NEW_PASSWORD=your-password
tiger db save-password svc-12345

# Save to specific storage location
tiger db save-password svc-12345 --password your-password --password-storage pgpass
tiger db save-password svc-12345 --password your-password --password-storage keyring

# Remove password from configured storage
tiger db remove-password svc-12345
tiger db remove-password svc-12345 --role readonly
```

**Return Codes for test-connection:**
The `test-connection` command follows `pg_isready` conventions:
- `0`: Server is accepting connections normally
- `1`: Server is rejecting connections (e.g., during startup)
- `2`: No response to connection attempt (server unreachable)
- `3`: No attempt made (e.g., invalid parameters)

**Authentication:**
The `connect` and `psql` commands automatically handle authentication using:
1. Stored password from configured storage method (keyring, ~/.pgpass file, or none based on --password-storage setting)
2. `PGPASSWORD` environment variable
3. Interactive password prompt (if neither above is available)

**Advanced psql Usage:**
The `connect` and `psql` commands support passing additional flags directly to the psql client using the `--` separator. Any flags after `--` are passed through to psql unchanged, allowing full access to psql's functionality while maintaining tiger's connection and authentication handling.

**Options:**
- `--pooled`: Use connection pooling (for connection-string command)
- `--role`: Database role to use (default: tsdbadmin)
- `--password`: Password to save (for save-password command). When flag is provided with no value, prompts interactively. When flag is not provided at all, uses TIGER_NEW_PASSWORD environment variable if set.
- `-t, --timeout`: Timeout for test-connection (accepts any duration format from Go's time.ParseDuration: "3s", "30s", "1m", etc.) (default: 3s, set to 0 to disable)

### High-Availability Management

#### `tiger ha`
Manage high-availability replicas for fault tolerance.

**Subcommands:**
- `get`: Show current HA configuration (aliases: `describe`, `show`)
- `set`: Set HA configuration level

**Examples:**
```bash
# Show current HA configuration
tiger ha get svc-12345

# Set HA level
tiger ha set svc-12345 --level none
tiger ha set svc-12345 --level high
tiger ha set svc-12345 --level highest-performance
tiger ha set svc-12345 --level highest-dataintegrity
```

**HA Levels:**
- `none`: No high-availability replicas (0 async, 0 sync)
- `high`: Single async replica in different AZ (1 async, 0 sync) - cost efficient, best for production apps
- `highest-performance`: Two async replicas in different AZs (2 async, 0 sync) - highest availability with queryable HA system, best for critical apps
- `highest-dataintegrity`: One sync + one async replica (1 async, 1 sync) - sync replica identical to primary, best for zero data loss tolerance

**Options:**
- `--level`: HA configuration level (none, high, highest-performance, highest-dataintegrity)

### Read Replica Sets Management

#### `tiger read-replica`
Manage read replica sets for scaling read workloads.

**Subcommands:**
- `list`: List all read replica sets
- `get`: Show replica set details (aliases: `describe`, `show`)
- `create`: Create a read replica set
- `delete`: Delete a replica set
- `resize`: Resize replica set resources
- `enable-pooler`: Enable connection pooler
- `disable-pooler`: Disable connection pooler
- `set-environment`: Set environment type
- `attach-vpc`: Attach replica set to a VPC
- `detach-vpc`: Detach replica set from VPC

**Examples:**
```bash
# List read replica sets
tiger read-replica list svc-12345

# Create read replica set
tiger read-replica create svc-12345 \
  --name "reporting-replica" \
  --nodes 2 \
  --cpu 500m \
  --memory 2GB

# Create read replica set in specific VPC
tiger read-replica create svc-12345 \
  --name "vpc-replica" \
  --nodes 1 \
  --cpu 500m \
  --memory 2GB \
  --vpc-id vpc-67890

# Resize replica set
tiger read-replica resize replica-67890 --nodes 3 --cpu 1000m

# Enable connection pooler
tiger read-replica enable-pooler replica-67890

# Attach/detach VPC
tiger read-replica attach-vpc replica-67890 --vpc-id vpc-12345
tiger read-replica detach-vpc replica-67890 --vpc-id vpc-12345
```

**Options:**
- `--name`: Replica set name
- `--nodes`: Number of nodes in replica set
- `--cpu`: CPU allocation per node
- `--memory`: Memory allocation per node
- `--vpc-id`: VPC ID for creation or attach/detach operations

### VPC Management

#### `tiger vpc`
Manage Virtual Private Clouds.

**Subcommands:**
- `list`: List all VPCs
- `get`: Show VPC details (aliases: `describe`, `show`)
- `create`: Create a new VPC
- `delete`: Delete a VPC
- `rename`: Rename a VPC
- `attach-service`: Attach a service to this VPC
- `detach-service`: Detach a service from this VPC
- `list-services`: List services attached to this VPC
- `peering`: Manage VPC peering connections

**Examples:**
```bash
# List VPCs
tiger vpc list

# Create VPC
tiger vpc create \
  --name "production-vpc" \
  --cidr "10.0.0.0/16" \
  --region us-east-1

# Show VPC details
tiger vpc get vpc-12345

# Attach/detach services
tiger vpc attach-service vpc-12345 --service-id svc-67890
tiger vpc detach-service vpc-12345 --service-id svc-67890

# List services in VPC
tiger vpc list-services vpc-12345

# Manage VPC peering (see VPC Peering Management section for details)
tiger vpc peering list vpc-12345
```

**Options:**
- `--name`: VPC name
- `--cidr`: CIDR block
- `--region`: Region code
- `--service-id`: Service ID for attach/detach operations

### VPC Peering Management

#### `tiger vpc peering`
Manage VPC peering connections for a specific VPC.

**Subcommands:**
- `list`: List all peering connections for a VPC
- `get`: Show details of a specific peering connection (aliases: `describe`, `show`)
- `create`: Create a new peering connection
- `delete`: Delete a peering connection

**Examples:**
```bash
# List all peering connections for a VPC
tiger vpc peering list vpc-12345

# Show details of a specific peering connection
tiger vpc peering get vpc-12345 peer-67890

# Create a new peering connection
tiger vpc peering create vpc-12345 \
  --peer-account-id acc-54321 \
  --peer-vpc-id vpc-abcdef \
  --peer-region us-east-1

# Delete a peering connection
tiger vpc peering delete vpc-12345 peer-67890
```

**Options:**
- `--peer-account-id`: Account ID of the peer VPC
- `--peer-vpc-id`: VPC ID of the peer VPC
- `--peer-region`: Region code of the peer VPC

### Utility Commands

#### `tiger completion`
Generate shell completion scripts.

**Examples:**
```bash
# Generate bash completion
tiger completion bash > /etc/bash_completion.d/tiger

# Generate zsh completion
tiger completion zsh > ~/.zsh/completions/_tiger
```

#### `tiger config`
Manage CLI configuration.

**Subcommands:**
- `show`: Show current configuration
- `set`: Set configuration value (modifies ~/.config/tiger/config.yaml)
- `unset`: Remove configuration value (modifies ~/.config/tiger/config.yaml)
- `reset`: Reset to defaults (overwrites ~/.config/tiger/config.yaml)

**Examples:**
```bash
# Show config
tiger config show

# Set default project
tiger config set project_id proj-12345

# Set default service
tiger config set service_id svc-12345

# Set output format
tiger config set output json

# Remove a specific setting
tiger config unset service_id

# Reset all settings to defaults
tiger config reset
```

## Exit Codes

- `0`: Success
- `1`: General error
- `2`: Operation timeout (wait-timeout exceeded for service operations) or connection timeout (for db test-connection)
- `3`: Invalid parameters
- `4`: Authentication error
- `5`: Permission denied
- `6`: Service not found
- `7`: Update available (for explicit `version --check`)

## Output Formats

### JSON
```json
{
  "id": "svc-12345",
  "name": "production-db",
  "status": "running",
  "tier": "production"
}
```

### YAML
```yaml
id: svc-12345
name: production-db
status: running
tier: production
```

### Table (Default)
```
ID         NAME           STATUS    TIER
svc-12345  production-db  running   production
```

## Error Handling

Errors are returned with descriptive messages and appropriate exit codes:

```bash
$ tiger service get invalid-id
Error: Service 'invalid-id' not found in project 'proj-12345'
Use 'tiger service list' to see available services.
```

## Configuration Precedence

1. Command-line flags
2. Environment variables
3. Configuration file
4. Default values

**Note:** The `api_url` configuration is intentionally not exposed as a CLI flag (`--api-url`). It can only be configured via environment variable (`TIGER_API_URL`), configuration file, or the config command (`tiger config set api_url <url>`). This is primarily intended for internal debugging and development use.

## Go Library Dependencies

### Core Libraries

**CLI Framework:**
- [`github.com/spf13/cobra`](https://github.com/spf13/cobra) - Powerful CLI framework with subcommands, flags, and built-in help
- [`github.com/spf13/pflag`](https://github.com/spf13/pflag) - POSIX/GNU-style flag library (included with Cobra)

**API Client Generation:**
- [`github.com/oapi-codegen/oapi-codegen/v2`](https://github.com/oapi-codegen/oapi-codegen) - OpenAPI 3.0 client generation with clean, idiomatic Go code
- [`github.com/oapi-codegen/runtime`](https://github.com/oapi-codegen/runtime) - Runtime types and utilities for generated client code
- [`net/http`](https://pkg.go.dev/net/http) - Standard library HTTP client (used by generated code)

**Configuration Management:**
- [`github.com/spf13/viper`](https://github.com/spf13/viper) - Configuration management with YAML/JSON support and environment variables
- [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3) - YAML parsing (included with Viper)

**Secure Credential Storage:**
- [`github.com/zalando/go-keyring`](https://github.com/zalando/go-keyring) - System keyring integration (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- [`os` package](https://pkg.go.dev/os) - File permissions for fallback storage

**Output Formatting:**
- [`encoding/json`](https://pkg.go.dev/encoding/json) - Standard library JSON encoding
- [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3) - YAML output formatting
- [`github.com/olekukonko/tablewriter`](https://github.com/olekukonko/tablewriter) - ASCII table formatting
- [`github.com/logrusorgru/aurora/v4`](https://github.com/logrusorgru/aurora) - Terminal colors and formatting

**Database Connectivity:**
- [`github.com/jackc/pgx/v5`](https://github.com/jackc/pgx) - Modern PostgreSQL driver with native Go implementation

**Shell Completion:**
- Built into `github.com/spf13/cobra` - Automatic completion generation for bash, zsh, fish, PowerShell

### Optional Enhancement Libraries

**Logging:**
- [`github.com/sirupsen/logrus`](https://github.com/sirupsen/logrus) - Structured logging for debug mode
- [`log/slog`](https://pkg.go.dev/log/slog) - Alternative: Go 1.21+ structured logging

**Progress Indicators:**
- [`github.com/schollz/progressbar/v3`](https://github.com/schollz/progressbar/v3) - Progress bars for long-running operations
- [`github.com/briandowns/spinner`](https://github.com/briandowns/spinner) - Spinners for waiting states

**Validation:**
- [`github.com/go-playground/validator/v10`](https://github.com/go-playground/validator/v10) - Input validation (already in use)

### Library Selection Rationale

1. **Cobra + Viper**: Industry standard for Go CLI applications, excellent integration, robust feature set
2. **oapi-codegen**: Generates clean, idiomatic Go code from OpenAPI 3.0 specs with minimal dependencies
3. **go-keyring**: Referenced in the spec, mature library with cross-platform support
4. **Standard library HTTP**: Works seamlessly with generated OpenAPI clients, reduces dependencies  
5. **tablewriter**: Clean ASCII table output, good for terminal display
6. **jackc/pgx**: Modern PostgreSQL driver with better performance and active maintenance (replaces deprecated lib/pq)

## Examples and Common Workflows

### Initial Setup
```bash
# Authenticate
tiger auth login

# List available resources
tiger service list
```

### Creating a Production Environment
```bash
# Create VPC
tiger vpc create --name "prod-vpc" --cidr "10.0.0.0/16" --region us-east-1

# Create production service
tiger service create \
  --name "production-db" \
  --type timescaledb \
  --region us-east-1 \
  --cpu 2 \
  --memory 8GB \
  --replicas 2
```

### Database Operations
```bash
# Connect to database
tiger db connect svc-12345

# Test database connectivity
tiger db test-connection svc-12345

# Get connection string
tiger db connection-string svc-12345
```

### Monitoring and Maintenance
```bash
# Check service status
tiger service get svc-12345

# Update service password
tiger service update-password svc-12345 --password new-secure-password

# Show current configuration
tiger config show
```

### Service ID Parameter Patterns

Commands follow consistent patterns for specifying service IDs:

**Single-service commands** (verbs acting on one service):
- Use positional `<service-id>` as the canonical parameter
- Support `--service-id` flag as an alias/override
- Examples: `tiger service get <service-id>`, `tiger db connect <service-id>`

**Global context commands** (acting on other resources with service as secondary):
- Use `--service-id` flag as the canonical parameter  
- Examples: `tiger vpc attach-service <vpc-id> --service-id <service-id>`
