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

- `TIGER_API_KEY`: TigerData API key for authentication
- `TIGER_API_URL`: Base URL for TigerData API (default: https://api.tigerdata.com/public/v1)
- `TIGER_PROJECT_ID`: Default project ID to use
- `TIGER_SERVICE_ID`: Default service ID to use
- `TIGER_CONFIG_DIR`: Configuration directory (default: ~/.config/tiger)
- `TIGER_OUTPUT`: Default output format (json, yaml, table)
- `TIGER_ANALYTICS`: Enable/disable usage analytics (true/false)

### Configuration File

Location: `~/.config/tiger/config.yaml`

```yaml
api_url: https://api.tigerdata.com/public/v1
project_id: your-default-project-id
service_id: your-default-service-id  # optional
output: table
analytics: true
```

### Global Options

- `-o, --output`: Set output format (json, yaml, table)
- `--config-dir`: Path to configuration directory
- `--api-key`: Specify TigerData API key
- `--project-id`: Specify project ID
- `--service-id`: Specify service ID
- `--analytics`: Toggle analytics collection
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
- `tiger auth whoami` - Show current user

**Core Service Management:**
- `tiger services list` - List all services
- `tiger services show` - Show service details
- `tiger services create` - Create new services
- `tiger services update-password` - Update service master password

**Database Operations:**
- `tiger db connect` / `tiger db psql` - Connect to databases
- `tiger db connection-string` - Get connection strings
- `tiger db test-connection` - Test connectivity

**Configuration:**
- `tiger config show` - Show current configuration
- `tiger config set` - Set configuration values

**Future v1+ Commands:**
- Service lifecycle (start/stop/restart) - pending API endpoints
- Service deletion with confirmation
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
- `whoami`: Show current user information

**Examples:**
```bash
# Login with API key
tiger auth login --api-key YOUR_API_KEY

# Login with token from environment
tiger auth login

# Show current user
tiger auth whoami

# Logout
tiger auth logout
```

**Authentication Methods:**
1. `--api-key` flag: Provide API key directly
2. `TIGER_API_KEY` environment variable
3. Interactive prompt for API key

**Login Process:**
When using `tiger auth login`, the CLI will:
1. Store the API key securely using system keyring or file fallback
2. Retrieve the project ID associated with the token from the API
3. Store the project ID in `~/.config/tiger/config.yaml` as the default project

**API Key Storage:**
The API key is stored securely using:
1. **System keyring** (preferred): Uses [go-keyring](https://github.com/zalando/go-keyring) for secure storage in system credential managers (macOS Keychain, Windows Credential Manager, Linux Secret Service)  
2. **File fallback**: If keyring is unavailable, stores in `~/.config/tiger/api-key` with restricted file permissions (600)

**Options:**
- `--api-key`: API key for authentication

### Service Management

#### `tiger services`
Manage database services.

**Subcommands:**
- `list`: List all services
- `show`: Show service details
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

**Examples:**
```bash
# List services
tiger services list

# Show service details
tiger services show svc-12345

# Create a TimescaleDB service
tiger services create \
  --name "production-db" \
  --type timescaledb \
  --region google-us-central1 \
  --cpu 2000m \
  --memory 8GB \
  --replicas 2

# Create a PostgreSQL service (waits for ready by default)
tiger services create \
  --name "postgres-db" \
  --type postgres \
  --region google-europe-west1 \
  --cpu 1 \
  --memory 4GB \
  --replicas 1

# Create service without waiting
tiger services create \
  --name "quick-service" \
  --type timescaledb \
  --region google-us-central1 \
  --cpu 1000m \
  --memory 2GB \
  --replicas 1 \
  --no-wait

# Create service with custom timeout
tiger services create \
  --name "patient-service" \
  --type postgres \
  --region google-europe-west1 \
  --cpu 2000m \
  --memory 8GB \
  --replicas 2 \
  --timeout 60

# Resize service
tiger services resize svc-12345 --cpu 4 --memory 16GB

# Start/stop service
tiger services start svc-12345
tiger services stop svc-12345

# Attach/detach VPC
tiger services attach-vpc svc-12345 --vpc-id vpc-67890
tiger services detach-vpc svc-12345 --vpc-id vpc-67890

# Enable/disable connection pooling
tiger services enable-pooler svc-12345
tiger services disable-pooler svc-12345

# Update service password
tiger services update-password svc-12345 --password new-secure-password

# Set default service
tiger services set-default svc-12345
```

**Options:**
- `--name`: Service name (required)
- `--type`: Service type (timescaledb, postgres, vector) - default: timescaledb
- `--region`: Region code (required)
- `--cpu`: CPU allocation - supports cores (e.g., "2") or millicores (e.g., "2000m")
- `--memory`: Memory allocation with units (e.g., "8GB", "4096MB")
- `--replicas`: Number of high-availability replicas (default: 1)
- `--vpc-id`: VPC ID for attach/detach operations
- `--save-password`: Save password to ~/.pgpass file (default: true)
- `--no-save-password`: Don't save password to ~/.pgpass file
- `--set-default`: Set this service as the default service (default: true)
- `--no-set-default`: Don't set this service as the default service
- `--wait`: Wait for service to be ready (default: true)
- `--no-wait`: Don't wait for service to be ready, return immediately
- `--timeout`: Timeout for waiting in minutes (default: 30)
- `--password`: New password (for update-password command)

**Default Behavior:**
- **Password Management**: By default, service creation will save the generated password to `~/.pgpass` for automatic authentication. Use `--no-save-password` to disable this behavior and manage passwords manually.
- **Default Service**: By default, newly created services will be set as the default service in your configuration. Use `--no-set-default` to disable this behavior and keep your current default service unchanged.
- **Wait for Ready**: By default, the command will wait for the service to be ready before returning, displaying "Waiting for service to be ready..." every 10 seconds. Use `--no-wait` to return immediately after creation request is accepted, or `--timeout` to specify a custom timeout period.

### Database Operations

#### `tiger db`
Database-specific operations and management.

**Subcommands:**
- `connect`: Connect to a database
- `psql`: Connect to a database (alias for connect)
- `connection-string`: Get connection string for a service
- `test-connection`: Test database connectivity
- `save-password`: Save password to ~/.pgpass file
- `remove-password`: Remove password from ~/.pgpass file

**Examples:**
```bash
# Connect to database
tiger db connect svc-12345
# or use psql alias
tiger db psql svc-12345

# Get connection string
tiger db connection-string svc-12345
# Get pooled connection string
tiger db connection-string svc-12345 --pooled

# Test database connectivity
tiger db test-connection svc-12345

# Save password to .pgpass
tiger db save-password svc-12345 --password your-password
tiger db save-password svc-12345 --password your-password --role readonly

# Remove password from .pgpass
tiger db remove-password svc-12345
tiger db remove-password svc-12345 --role readonly
```

**Authentication:**
The `connect` and `psql` commands automatically handle authentication using:
1. `~/.pgpass` file (if password was saved during service creation)
2. `PGPASSWORD` environment variable
3. Interactive password prompt (if neither above is available)

**Options:**
- `--pooled`: Use connection pooling (for connection-string command)
- `--role`: Database role to use (default: tsdbadmin)
- `--password`: Password to save (for save-password command)

### High-Availability Management

#### `tiger ha`
Manage high-availability replicas for fault tolerance.

**Subcommands:**
- `show`: Show current HA configuration
- `set`: Set HA configuration level

**Examples:**
```bash
# Show current HA configuration
tiger ha show svc-12345

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

#### `tiger read-replicas`
Manage read replica sets for scaling read workloads.

**Subcommands:**
- `list`: List all read replica sets
- `show`: Show replica set details
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
tiger read-replicas list svc-12345

# Create read replica set
tiger read-replicas create svc-12345 \
  --name "reporting-replica" \
  --nodes 2 \
  --cpu 500m \
  --memory 2GB

# Create read replica set in specific VPC
tiger read-replicas create svc-12345 \
  --name "vpc-replica" \
  --nodes 1 \
  --cpu 250m \
  --memory 1GB \
  --vpc-id vpc-67890

# Resize replica set
tiger read-replicas resize replica-67890 --nodes 3 --cpu 1000m

# Enable connection pooler
tiger read-replicas enable-pooler replica-67890

# Attach/detach VPC
tiger read-replicas attach-vpc replica-67890 --vpc-id vpc-12345
tiger read-replicas detach-vpc replica-67890 --vpc-id vpc-12345
```

**Options:**
- `--name`: Replica set name
- `--nodes`: Number of nodes in replica set
- `--cpu`: CPU allocation per node
- `--memory`: Memory allocation per node
- `--vpc-id`: VPC ID for creation or attach/detach operations

### VPC Management

#### `tiger vpcs`
Manage Virtual Private Clouds.

**Subcommands:**
- `list`: List all VPCs
- `show`: Show VPC details
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
tiger vpcs list

# Create VPC
tiger vpcs create \
  --name "production-vpc" \
  --cidr "10.0.0.0/16" \
  --region google-us-central1

# Show VPC details
tiger vpcs show vpc-12345

# Attach/detach services
tiger vpcs attach-service vpc-12345 --service-id svc-67890
tiger vpcs detach-service vpc-12345 --service-id svc-67890

# List services in VPC
tiger vpcs list-services vpc-12345

# Manage VPC peering (see VPC Peering Management section for details)
tiger vpcs peering list vpc-12345
```

**Options:**
- `--name`: VPC name
- `--cidr`: CIDR block
- `--region`: Region code
- `--service-id`: Service ID for attach/detach operations

### VPC Peering Management

#### `tiger vpcs peering`
Manage VPC peering connections for a specific VPC.

**Subcommands:**
- `list`: List all peering connections for a VPC
- `show`: Show details of a specific peering connection
- `create`: Create a new peering connection
- `delete`: Delete a peering connection

**Examples:**
```bash
# List all peering connections for a VPC
tiger vpcs peering list vpc-12345

# Show details of a specific peering connection
tiger vpcs peering show vpc-12345 peer-67890

# Create a new peering connection
tiger vpcs peering create vpc-12345 \
  --peer-account-id acc-54321 \
  --peer-vpc-id vpc-abcdef \
  --peer-region aws-us-east-1

# Delete a peering connection
tiger vpcs peering delete vpc-12345 peer-67890
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
- `2`: Authentication error
- `3`: Resource not found
- `4`: Permission denied
- `5`: Service unavailable
- `6`: Invalid configuration

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
$ tiger services show invalid-id
Error: Service 'invalid-id' not found in project 'proj-12345'
Use 'tiger services list' to see available services.
```

## Configuration Precedence

1. Command-line flags
2. Environment variables
3. Configuration file
4. Default values

## Go Library Dependencies

### Core Libraries

**CLI Framework:**
- [`github.com/spf13/cobra`](https://github.com/spf13/cobra) - Powerful CLI framework with subcommands, flags, and built-in help
- [`github.com/spf13/pflag`](https://github.com/spf13/pflag) - POSIX/GNU-style flag library (included with Cobra)

**API Client Generation:**
- [`github.com/go-swagger/go-swagger`](https://github.com/go-swagger/go-swagger) - OpenAPI client generation (integrates with existing go-openapi dependencies)
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
- [`github.com/lib/pq`](https://github.com/lib/pq) - PostgreSQL driver for database connections
- [`database/sql`](https://pkg.go.dev/database/sql) - Standard library database interface

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
2. **go-swagger**: Leverages existing go-openapi dependencies in the repo, generates clean client code
3. **go-keyring**: Referenced in the spec, mature library with cross-platform support
4. **Standard library HTTP**: Works seamlessly with generated swagger clients, reduces dependencies  
5. **tablewriter**: Clean ASCII table output, good for terminal display
6. **lib/pq**: Most popular PostgreSQL driver for Go, stable and well-maintained

## Examples and Common Workflows

### Initial Setup
```bash
# Authenticate
tiger auth login

# Set default project
tiger projects set-default proj-12345

# List available resources
tiger services list
tiger vpcs list
```

### Creating a Production Environment
```bash
# Create VPC
tiger vpcs create --name "prod-vpc" --cidr "10.0.0.0/16" --region google-us-central1

# Create production service
tiger services create \
  --name "production-db" \
  --tier "production" \
  --vpc-id vpc-12345 \
  --region google-us-central1 \
  --postgres-version 15

# Create read replica
tiger replicas create svc-67890 \
  --name "read-replica-east" \
  --region google-us-east1
```

### Database Operations
```bash
# Connect to database
tiger db connect svc-12345

# Create and apply migration
tiger migrations new --name "add_index_on_users"
# Edit the migration file
tiger migrations up svc-12345

# Backup database
tiger db dump svc-12345 --output backup-$(date +%Y%m%d).sql
```

### Monitoring and Maintenance
```bash
# Check service status
tiger services show svc-12345

# Inspect performance
tiger inspect slow-queries svc-12345
tiger inspect bloat svc-12345

# View recent operations
tiger operations list --limit 10
```