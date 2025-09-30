# TigerData MCP Tool Description Optimization

## Overview
This document analyzes the verbosity of TigerData MCP tool descriptions and provides streamlined alternatives optimized for LLM tool selection.

## Key Principles for LLM-Optimized Descriptions
1. Front-load critical information (what the tool does)
2. Remove redundant phrases like "This tool" at the beginning
3. Eliminate "Perfect for" sections - LLMs don't need use case suggestions
4. Keep only essential operational details (wait behavior, security notes)
5. Avoid explanatory text that doesn't affect tool selection

---

## Tool: service_list

### Original Description
```
List all database services in your current TigerData project.

This tool retrieves a complete list of database services with their basic information including status, type, region, and resource allocation. Use this to get an overview of all your services before performing operations on specific services.

Perfect for:
- Getting an overview of your database infrastructure
- Finding service IDs for other operations
- Checking service status and resource allocation
- Discovering services across different regions
```

### Suggested Description
```
List all database services in current TigerData project. Returns services with status, type, region, and resource allocation.
```

### Rationale
- Removed: "This tool retrieves" (redundant), "Use this to get an overview" (obvious), entire "Perfect for" section (LLMs infer usage from function)
- Kept: Core function and return data

---

## Tool: service_show

### Original Description
```
Show detailed information about a specific database service.

This tool provides comprehensive information about a service including connection endpoints, replica configuration, resource allocation, creation time, and current operational status. Essential for debugging, monitoring, and connection management.

Perfect for:
- Getting connection endpoints (direct and pooled)
- Checking detailed service configuration
- Monitoring service health and status
- Obtaining service specifications for scaling decisions
```

### Suggested Description
```
Get detailed information for a specific database service. Returns connection endpoints, replica configuration, resource allocation, creation time, and status.
```

### Rationale
- Removed: "This tool provides" (redundant), "Essential for" (subjective), entire "Perfect for" section
- Kept: Specific data returned (important for LLM to know what info is available)

---

## Tool: service_create

### Original Description
```
Create a new database service in TigerData Cloud.

This tool provisions a new database service with specified configuration including service type, compute resources, region, and high availability options.

By default, this tool returns immediately after the creation request is accepted. The service will continue provisioning in the background and may not be ready for connections yet.

Set 'wait: true' to block until the service is fully ready for connections. Use 'timeout' to control how long to wait (only applies when wait=true).

IMPORTANT: This operation incurs costs and creates billable resources. Always confirm requirements before proceeding.

Perfect for:
- Setting up new database infrastructure
- Creating development or production environments
- Provisioning databases with specific resource requirements
- Establishing services in different geographical regions
```

### Suggested Description
```
Create a new database service in TigerData Cloud with specified type, compute resources, region, and HA options.

Default: Returns immediately (service provisions in background).
wait=true: Blocks until service ready.
timeout: Wait duration in minutes (with wait=true).

WARNING: Creates billable resources.
```

### Rationale
- Removed: "This tool provisions" (redundant), entire "Perfect for" section, verbose explanation of wait behavior
- Kept: Critical operational behavior (async by default), wait options, billing warning
- Reformatted: More scannable format for parameters

---

## Tool: service_update_password

### Original Description
```
Update the master password for the 'tsdbadmin' user of a database service.

This tool changes the master database password used for the default administrative user. The new password will be required for all future database connections. Existing connections may be terminated.

SECURITY NOTE: Ensure new passwords are strong and stored securely. Password changes take effect immediately.

Perfect for:
- Password rotation for security compliance
- Recovering from compromised credentials
- Setting initial passwords for new services
- Meeting organizational security policies
```

### Suggested Description
```
Update master password for 'tsdbadmin' user of a database service. Takes effect immediately. May terminate existing connections.
```

### Rationale
- Removed: Redundant explanation of what password update means, security advice (LLMs don't need this), entire "Perfect for" section
- Kept: User affected ('tsdbadmin'), immediate effect, connection termination warning

---

## Implementation in service_tools.go

To implement these changes, update the Description fields in `/Users/cevian/Development/tiger-cli/internal/tiger/mcp/service_tools.go`:

### Line 186-194 (service_list)
```go
Description: `List all database services in current TigerData project. Returns services with status, type, region, and resource allocation.`,
```

### Line 207-215 (service_show)
```go
Description: `Get detailed information for a specific database service. Returns connection endpoints, replica configuration, resource allocation, creation time, and status.`,
```

### Line 228-242 (service_create)
```go
Description: `Create a new database service in TigerData Cloud with specified type, compute resources, region, and HA options.

Default: Returns immediately (service provisions in background).
wait=true: Blocks until service ready.
timeout: Wait duration in minutes (with wait=true).

WARNING: Creates billable resources.`,
```

### Line 256-266 (service_update_password)
```go
Description: `Update master password for 'tsdbadmin' user of a database service. Takes effect immediately. May terminate existing connections.`,
```

---

## Summary of Reductions

| Tool | Original Length | Suggested Length | Reduction |
|------|----------------|------------------|-----------|
| service_list | 453 chars | 106 chars | 77% reduction |
| service_show | 467 chars | 128 chars | 73% reduction |
| service_create | 715 chars | 207 chars | 71% reduction |
| service_update_password | 444 chars | 108 chars | 76% reduction |

**Total character reduction: ~74%** while maintaining all essential information for LLM tool selection.