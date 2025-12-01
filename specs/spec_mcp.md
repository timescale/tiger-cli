# Tiger MCP Server Specification

## Overview

The Tiger MCP (Model Context Protocol) Server provides programmatic access to Tiger Cloud platform resources through Claude and other AI assistants. It mirrors the functionality of the Tiger CLI and is integrated directly into the CLI binary for seamless operation.

The MCP server is written in Go and launched via the Tiger CLI, sharing the same authentication, configuration, and API client.

## Design Decisions

### Dynamic Configuration Loading

Each MCP tool call dynamically creates the API client and loads configuration at execution time, rather than initializing these once when the MCP server starts. This design ensures that configuration changes (API keys, project IDs, etc.) take effect immediately for subsequent tool calls without requiring users to restart the MCP server or reconnect their AI assistant. Users can run `tiger auth login` to update authentication or `tiger config set` to modify other configuration values and see changes reflected instantly in their AI interactions.

## v0 Tool Priority

For the initial v0 release, implement these essential tools first:

**Core Service Management:**
- `service_list` - List all services
- `service_get` - Get service details
- `service_create` - Create new services
- `service_fork` - Fork existing services
- `service_start` - Start stopped services
- `service_stop` - Stop running services
- `service_delete` - Delete services (with confirmation, 24-hour safe delete) - Maybe not v0
- `service_update_password` - Update service master password

**Database Operations:**
- `db_connection_string` - Get connection strings
- `db_execute_query` - Execute SQL queries
- `db_test_connection` - Test connectivity

**Future v1+ Tools:**
- Service restart - pending API endpoint
- HA management tools
- Read replica management
- Basic VPC management
- VPC peering management
- Advanced service operations (resize, pooler, VPC attach/detach)

## Configuration

The recommended approach is to use the `tiger mcp install` command, which automatically configures the Tiger MCP server for your AI assistant. See the CLI MCP Commands section below for details.

Alternatively, for manual configuration, the Tiger MCP server can be added to your AI assistant's configuration file with the following settings:

```json
{
  "mcpServers": {
    "tiger": {
      "command": "tiger",
      "args": ["mcp", "start"]
    }
  }
}
```

The MCP server will automatically use the CLI's stored authentication and configuration.

### CLI MCP Commands

#### `tiger mcp install <editor>`
Install and configure the Tiger MCP server for a specific editor or AI assistant. This command automates the configuration process by modifying the appropriate configuration files.

**Supported Editors:**
- `claude-code`: Configure for Claude Code
- `cursor`: Configure for Cursor IDE
- `windsurf`: Configure for Windsurf editor
- `codex`: Configure for Codex
- `gemini` or `gemini-cli`: Configure for Gemini CLI
- `vscode`, `code`, or `vs-code`: Configure for VS Code

**Options:**
- `--no-backup`: Skip creating backup of existing configuration (default: create backup)
- `--config-path`: Custom path to configuration file (overrides default locations)

**Examples:**
```bash
# Interactive editor selection
tiger mcp install

# Install for Claude Code
tiger mcp install claude-code

# Install for Cursor IDE
tiger mcp install cursor

# Install for Windsurf
tiger mcp install windsurf

# Install for VS Code
tiger mcp install vscode

# Install without creating backup
tiger mcp install claude-code --no-backup

# Use custom configuration file path
tiger mcp install claude-code --config-path ~/custom/config.json
```

**Behavior:**
- Automatically detects the appropriate configuration file location for the specified editor
- Creates configuration directory if it doesn't exist
- Creates backup of existing configuration file by default (use `--no-backup` to skip)
- Merges with existing MCP server configurations (doesn't overwrite other servers)
- Validates configuration after installation
- Provides clear success/failure feedback with next steps

**Configuration Format:**
The command adds the Tiger MCP server configuration using the appropriate format for each editor. Example configuration:
```json
{
  "mcpServers": {
    "tiger": {
      "command": "tiger",
      "args": ["mcp", "start"]
    }
  }
}
```

#### `tiger mcp start [transport]`
Start the MCP server with the specified transport. The server runs in the foreground and can be stopped with Ctrl+C.

**Transports:**
- `stdio` (default): Standard input/output transport for AI assistant integration
- `http`: HTTP server transport for web-based integrations

**Options for HTTP transport:**
- `--port`: Port to run HTTP server on (default: 8080, or a free port if 8080 is unavailable)
- `--host`: Host to bind to (default: localhost)

**Examples:**
```bash
# Start MCP server with stdio transport (default)
tiger mcp start
tiger mcp start stdio

# Start HTTP server for web integrations
tiger mcp start http
tiger mcp start http --port 3001
tiger mcp start http --port 8080 --host 0.0.0.0
```

**Notes:**
- The MCP server runs in the foreground and will continue until stopped with Ctrl+C or terminated by the calling process
- For HTTP transport, the server will print the listening address (including port) on startup for easy connection

## Available Tools

### Service Management

#### `service_list`
List all database services.

**Parameters:** None

**Returns:** Array of service objects with id, name, status, type, region, and resource information.

#### `service_get`
Get details of a specific service.

**Parameters:**
- `service_id` (string, required): Service ID to get
- `with_password` (boolean, optional): Include password in response and connection string - default: false

**Returns:** Detailed service object with configuration, endpoints, status, and connection string. When `with_password=true`, the response includes the password field and the password is embedded in the connection string. When `with_password=false` (default), the connection string is still included but without the password embedded.

#### `service_create`
Create a new database service.

**Parameters:**
- `name` (string, optional): Service name - auto-generated if not provided
- `addons` (array, optional): Addons to enable ("time-series", "ai", or empty array for PostgreSQL-only)
- `region` (string, optional): Region code
- `cpu_memory` (string, optional): CPU and memory allocation combination (e.g., "shared/shared", "0.5 CPU/2GB", "2 CPU/8GB")
- `replicas` (number, optional): Number of high-availability replicas - default: 0
- `wait` (boolean, optional): Wait for service to be ready (waits up to 10 minutes) - default: false
- `set_default` (boolean, optional): Set the newly created service as the default service for future commands - default: true
- `with_password` (boolean, optional): Include password in response and connection string - default: false

**Returns:** Service object with creation status, details, and connection string. When `with_password=true`, the response includes the initial password field and the password is embedded in the connection string. When `with_password=false` (default), the connection string is still included but without the password embedded.

**Note:** This tool automatically stores the database password using the same method as the CLI (keyring, pgpass file, etc.), regardless of the `with_password` parameter value.

#### `service_fork`
Fork an existing database service to create a new independent copy.

**Parameters:**
- `service_id` (string, required): Source service ID to fork
- `fork_strategy` (string, required): Fork strategy - must be one of:
  - `"NOW"`: Fork at the current database state (creates new snapshot or uses WAL replay)
  - `"LAST_SNAPSHOT"`: Fork at the last existing snapshot (faster fork)
  - `"PITR"`: Fork at a specific point in time (point-in-time recovery)
- `target_time` (string, optional): Target timestamp for point-in-time recovery in RFC3339 format (e.g., "2025-01-15T10:30:00Z"). Required when `fork_strategy` is `"PITR"`, forbidden otherwise.
- `name` (string, optional): Name for the forked service - auto-generated if not provided
- `cpu_memory` (string, optional): CPU and memory allocation combination (e.g., "0.5 CPU/2GB", "2 CPU/8GB"). If not specified, inherits from source service.
- `wait` (boolean, optional): Wait for forked service to be ready (waits up to 10 minutes) - default: false
- `set_default` (boolean, optional): Set the forked service as the default service for future commands - default: true
- `with_password` (boolean, optional): Include password in response and connection string - default: false

**Returns:** Forked service object with creation status, details, and connection string. When `with_password=true`, the response includes the initial password field and the password is embedded in the connection string. When `with_password=false` (default), the connection string is still included but without the password embedded.

**Note:** This tool automatically stores the database password using the same method as the CLI (keyring, pgpass file, etc.), regardless of the `with_password` parameter value.

**Warning:** Creates billable resources.

#### `service_delete`
Delete a database service.

**Parameters:**
- `service_id` (string, required): Service ID to delete
- `confirmed` (boolean, required): Confirmation that deletion is intended - must be true

**Returns:** Deletion confirmation with operation status.

#### `service_start`
Start a stopped database service.

**Parameters:**
- `service_id` (string, required): Service ID to start
- `wait` (boolean, optional): Wait for service to be fully started before returning (waits up to 10 minutes). Default is false (recommended) - only set to true if your next steps require connecting to or querying this database.

**Returns:** Operation status with current service status and message.

#### `service_stop`
Stop a running database service.

**Parameters:**
- `service_id` (string, required): Service ID to stop
- `wait` (boolean, optional): Wait for service to be fully stopped before returning (waits up to 10 minutes). Default is false (recommended) - only set to true if your next steps require confirmation that the service is stopped.

**Returns:** Operation status with current service status and message.

#### `service_restart`
Restart a service.

**Parameters:**
- `service_id` (string, required): Service ID to restart

**Returns:** Operation status.

#### `service_resize`
Resize service resources.

**Parameters:**
- `service_id` (string, required): Service ID to resize
- `cpu` (string, optional): New CPU allocation
- `memory` (string, optional): New memory allocation

**Returns:** Resize operation status.

#### `service_enable_pooler`
Enable connection pooling for a service.

**Parameters:**
- `service_id` (string, required): Service ID

**Returns:** Operation status.

#### `service_disable_pooler`
Disable connection pooling for a service.

**Parameters:**
- `service_id` (string, required): Service ID

**Returns:** Operation status.

#### `service_attach_vpc`
Attach a service to a VPC.

**Parameters:**
- `service_id` (string, required): Service ID
- `vpc_id` (string, required): VPC ID to attach to

**Returns:** Operation status.

#### `service_detach_vpc`
Detach a service from a VPC.

**Parameters:**
- `service_id` (string, required): Service ID
- `vpc_id` (string, required): VPC ID to detach from

**Returns:** Operation status.

#### `service_update_password`
Update the master password for a service.

**Parameters:**
- `service_id` (string, required): Service ID
- `password` (string, required): New password for the service

**Returns:** Operation status confirmation.

**Note:** This tool automatically stores the database password using the same method as the CLI (keyring, pgpass file, etc.).

### Database Operations

#### `db_connection_string`
Get connection string for a service.

**Parameters:**
- `service_id` (string, optional): Service ID (uses default if not provided)
- `pooled` (boolean, optional): Use connection pooling - default: false
- `role` (string, optional): Database role to use

**Returns:** Connection string for the database.

#### `db_test_connection`
Test database connectivity.

**Parameters:**
- `service_id` (string, optional): Service ID (uses default if not provided)

**Returns:** Connection test results with status and latency information.

#### `db_execute_query`
Execute a SQL query on a service database.

**Parameters:**
- `service_id` (string, required): Service ID
- `query` (string, required): SQL query to execute
- `parameters` (array, optional): Query parameters for parameterized queries. Values are substituted for $1, $2, etc. placeholders in the query.
- `timeout_seconds` (number, optional): Query timeout in seconds (default: 30)
- `role` (string, optional): Database role/username to connect as (default: tsdbadmin)
- `pooled` (boolean, optional): Use connection pooling (default: false)

**Returns:** Query results with rows, columns (including types), rows affected count, and execution metadata.

**Example Response:**
```json
{
  "columns": [
    {"name": "id", "type": "int4"},
    {"name": "name", "type": "text"},
    {"name": "created_at", "type": "timestamptz"}
  ],
  "rows": [
    [1, "example", "2024-01-01T00:00:00Z"],
    [2, "test", "2024-01-02T00:00:00Z"]
  ],
  "rows_affected": 2,
  "execution_time": "15.2ms"
}
```

**Note:**
- `rows_affected` returns the number of rows returned for SELECT queries, and the number of rows modified for INSERT/UPDATE/DELETE queries
- `columns` includes both the column name and PostgreSQL data type for each column
- Empty `rows` array for commands that don't return rows (INSERT, UPDATE, DELETE, DDL commands)
- For parity with `tiger db connect` command, supports custom roles and connection pooling

### High-Availability Management

#### `ha_get`
Get current HA configuration for a service.

**Parameters:**
- `service_id` (string, required): Service ID

**Returns:** Current HA configuration with replica counts and levels.

#### `ha_set`
Set HA configuration level for a service.

**Parameters:**
- `service_id` (string, required): Service ID
- `level` (string, required): HA level (none, high, highest-performance, highest-dataintegrity)

**Returns:** HA configuration update status.

### Read Replica Sets Management

#### `read_replica_list`
List all read replica sets for a service.

**Parameters:**
- `service_id` (string, required): Primary service ID

**Returns:** Array of read replica set objects.

#### `read_replica_get`
Get details of a specific read replica set.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID

**Returns:** Detailed replica set object.

#### `read_replica_create`
Create a new read replica set.

**Parameters:**
- `service_id` (string, required): Primary service ID
- `name` (string, required): Replica set name
- `nodes` (number, required): Number of nodes in replica set
- `cpu` (string, required): CPU allocation per node
- `memory` (string, required): Memory allocation per node
- `vpc_id` (string, optional): VPC ID to deploy in

**Returns:** Replica set creation status and details.

#### `read_replica_delete`
Delete a read replica set.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID to delete

**Returns:** Deletion confirmation.

#### `read_replica_resize`
Resize a read replica set.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID
- `nodes` (number, optional): New number of nodes
- `cpu` (string, optional): New CPU allocation per node
- `memory` (string, optional): New memory allocation per node

**Returns:** Resize operation status.

#### `read_replica_enable_pooler`
Enable connection pooler for a read replica set.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID

**Returns:** Operation status.

#### `read_replica_disable_pooler`
Disable connection pooler for a read replica set.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID

**Returns:** Operation status.

#### `read_replica_attach_vpc`
Attach a read replica set to a VPC.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID
- `vpc_id` (string, required): VPC ID

**Returns:** Operation status.

#### `read_replica_detach_vpc`
Detach a read replica set from a VPC.

**Parameters:**
- `replica_set_id` (string, required): Replica set ID
- `vpc_id` (string, required): VPC ID

**Returns:** Operation status.

### VPC Management

#### `vpc_list`
List all Virtual Private Clouds.

**Parameters:** None

**Returns:** Array of VPC objects with id, name, CIDR, and region information.

#### `vpc_get`
Get details of a specific VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID to get

**Returns:** Detailed VPC object with configuration and attached services.

#### `vpc_create`
Create a new VPC.

**Parameters:**
- `name` (string, required): VPC name
- `cidr` (string, required): CIDR block (e.g., "10.0.0.0/16")
- `region` (string, required): Region code

**Returns:** VPC creation status and details.

#### `vpc_delete`
Delete a VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID to delete

**Returns:** Deletion confirmation.

#### `vpc_rename`
Rename a VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID to rename
- `name` (string, required): New VPC name

**Returns:** Rename operation status.

#### `vpc_list_services`
List services attached to a VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID

**Returns:** Array of services attached to the VPC.

#### `vpc_attach_service`
Attach a service to a VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID
- `service_id` (string, required): Service ID to attach

**Returns:** Operation status.

#### `vpc_detach_service`
Detach a service from a VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID
- `service_id` (string, required): Service ID to detach

**Returns:** Operation status.

### VPC Peering Management

#### `vpc_peering_list`
List all peering connections for a VPC.

**Parameters:**
- `vpc_id` (string, required): VPC ID

**Returns:** Array of peering connection objects.

#### `vpc_peering_get`
Get details of a specific peering connection.

**Parameters:**
- `vpc_id` (string, required): VPC ID
- `peering_id` (string, required): Peering connection ID

**Returns:** Detailed peering connection object.

#### `vpc_peering_create`
Create a new VPC peering connection.

**Parameters:**
- `vpc_id` (string, required): VPC ID
- `peer_account_id` (string, required): Account ID of the peer VPC
- `peer_vpc_id` (string, required): VPC ID of the peer VPC
- `peer_region` (string, required): Region code of the peer VPC

**Returns:** Peering connection creation status and details.

#### `vpc_peering_delete`
Delete a VPC peering connection.

**Parameters:**
- `vpc_id` (string, required): VPC ID
- `peering_id` (string, required): Peering connection ID to delete

**Returns:** Deletion confirmation.

## Error Handling

All tools return structured error responses when operations fail:

```json
{
  "error": {
    "code": "SERVICE_NOT_FOUND",
    "message": "Service 'svc-invalid' not found in project 'proj-12345'",
    "details": {
      "service_id": "svc-invalid",
      "project_id": "proj-12345"
    }
  }
}
```

Common error codes:
- `AUTHENTICATION_ERROR`: Invalid or missing API key
- `SERVICE_NOT_FOUND`: Requested service does not exist
- `VPC_NOT_FOUND`: Requested VPC does not exist
- `PERMISSION_DENIED`: Insufficient permissions for operation
- `RESOURCE_CONFLICT`: Resource is in a state that prevents the operation
- `VALIDATION_ERROR`: Invalid parameters provided
- `TIMEOUT_ERROR`: Operation timed out
- `SERVICE_UNAVAILABLE`: Tiger Cloud API is temporarily unavailable

## Implementation Notes

- The MCP server is embedded within the Tiger CLI binary
- Shares the same API client library and configuration system as the CLI
- Uses the CLI's stored authentication (keyring or file-based credentials)
- Inherits the CLI's project ID from stored credentials and service ID from configuration
- Implements proper graceful shutdown and signal handling
- Uses structured logging compatible with the CLI logging system
- All tools are idempotent where possible
- Follows the same error handling patterns as CLI commands
