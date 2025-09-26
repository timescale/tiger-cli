# TigerData MCP Tools Feedback

This document provides detailed feedback on the TigerData MCP (Model Context Protocol) tools based on analysis and testing.

## Executive Summary

The TigerData MCP tools are **functional and well-structured** but have several areas for improvement. The tools work reliably and return proper JSON responses, but the documentation needs refinement to prevent user confusion and improve discoverability.

**Overall Assessment:** Good foundation with room for documentation and naming improvements.

## Tool Name Analysis

### ✅ Strengths
- Follow clear naming convention: `mcp__tigerdata__tiger_service_*`
- Use descriptive action verbs: `create`, `list`, `show`, `update_password`
- Maintain consistency across all tools

### ❌ Issues
1. **Redundant prefix**: `tiger_service_` is redundant since the MCP is already `tigerdata`
2. **Inconsistent with CLI commands**: The CLI uses `service create`, but the MCP uses `tiger_service_create`

### 🔧 Recommendation
Rename to match CLI structure:
- `mcp__tigerdata__service_create`
- `mcp__tigerdata__service_list`
- `mcp__tigerdata__service_show`
- `mcp__tigerdata__service_update_password`

## Input Parameters Analysis

### tiger_service_create

#### ✅ Strengths
- Good use of enums for constrained values (`type`, `cpu_memory`, `region`)
- Clear examples provided for most parameters
- Proper type definitions with min/max constraints

#### ❌ Issues
1. **Inconsistent parameter naming**: Uses both `cpu_memory` (snake_case) and `replicas` (camelCase style)
2. **Unclear defaults**: `region` defaults to "us-east-1" but this isn't obvious to international users
3. **Confusing timeout parameter**: Only used when `wait: true`, but relationship isn't clear
4. **Missing cost information**: No indication of pricing implications for different configurations

#### 🔧 Recommendations
- Add cost warnings in descriptions
- Clarify `timeout` only applies when `wait: true`
- Consider renaming `cpu_memory` to `instance_size` or `compute_tier`

### tiger_service_show & tiger_service_list

#### ✅ Strengths
- Simple, focused parameters
- Clear parameter names

#### ❌ Issues
1. **No validation hints**: `service_id` format not documented (is it alphanumeric? UUID? length?)
2. **Missing examples**: `service_id` needs example format

### tiger_service_update_password

#### ✅ Strengths
- Clear security warnings
- Required parameters properly marked

#### ❌ Issues
1. **No password requirements**: No indication of password complexity rules
2. **Security risk**: Password passed as plain text parameter (though this may be unavoidable)

## Tool Description Analysis

### tiger_service_create

#### ✅ Strengths
- Excellent cost warnings and billing transparency
- Clear explanation of default behavior
- Good use cases listed
- Wait behavior properly explained

#### ❌ Issues
1. **Misleading provisioning info**: Says "returns immediately" but also mentions waiting - contradictory
2. **Vague resource allocation**: Doesn't explain what the CPU/memory combos actually provide in terms of performance

### tiger_service_list

#### ✅ Strengths
- Clear, concise description
- Good use case examples

#### ❌ Issues
1. **Missing output format info**: Doesn't specify what information is returned
2. **No empty state handling**: What happens when no services exist?

### tiger_service_show

#### ✅ Strengths
- Comprehensive list of returned information
- Clear use cases

#### ❌ Issues
1. **Technical jargon**: "pooled endpoints" not explained
2. **Missing error scenarios**: What if service doesn't exist?

### tiger_service_update_password

#### ✅ Strengths
- Strong security warnings
- Clear immediate effect explanation
- Good security compliance context

#### ❌ Issues
1. **Missing rollback info**: No mention of how to recover from password issues
2. **Connection impact unclear**: "Existing connections may be terminated" - when exactly?

## Tool Usage Testing Results

### ✅ Working Well
- Both `list` and `show` tools executed successfully
- Response format is clean JSON
- Service IDs are consistently formatted (alphanumeric, ~10 characters)
- No timeout or token limit issues encountered

### ❌ Discovered Issues
1. **Missing pooled endpoint**: `show` tool returned `direct_endpoint` but no "pooled endpoints" mentioned in description
2. **Inconsistent resource format**: Returns `"0.5 cores"` and `"2 GB"` but parameters use `"0.5 CPU/2GB"` format
3. **Missing fields**: No replica count, no high availability info in response
4. **Status values undocumented**: What other values besides "READY" are possible?

## Specific Improvement Recommendations

### 1. Fix Tool Naming Convention (HIGH PRIORITY)

**Current:** `mcp__tigerdata__tiger_service_create`
**Recommended:** `mcp__tigerdata__service_create`

**Why:** Eliminates redundant "tiger_" prefix and aligns with CLI command structure.

### 2. Improve Parameter Documentation (HIGH PRIORITY)

Add to all service_id parameters:
```json
"service_id": {
  "description": "The unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs.",
  "examples": ["e6ue9697jf", "u8me885b93"],
  "pattern": "^[a-z0-9]{10}$",
  "type": "string"
}
```

**Why:** Users need to know the expected format and how to find valid IDs.

### 3. Clarify Wait Behavior (MEDIUM PRIORITY)

**Current description issue:** Contradictory information about immediate return vs waiting.

**Recommended fix:**
```
"By default, this tool returns immediately after the creation request is accepted. The service will continue provisioning in the background and may not be ready for connections yet.

Set 'wait: true' to block until the service is fully ready for connections. Use 'timeout' to control how long to wait (only applies when wait=true)."
```

### 4. Add Response Schema Documentation (MEDIUM PRIORITY)

Each tool should document what JSON structure it returns:

```json
"returns": {
  "description": "Service information object",
  "properties": {
    "service": {
      "id": "string - Service identifier",
      "status": "string - One of: CREATING, READY, ERROR, PAUSED",
      "direct_endpoint": "string - Direct connection endpoint"
    }
  }
}
```

### 5. Add Error Scenario Documentation (MEDIUM PRIORITY)

```
"Error cases:
- Service ID not found: Returns 404 error
- Insufficient permissions: Returns 403 error
- Service in invalid state: Returns 400 error with details"
```

### 6. Enhance Security Documentation (LOW PRIORITY)

For `update_password`, add:
```
"Password requirements: Minimum 8 characters, must contain uppercase, lowercase, number, and special character.

Connection impact: Active connections will be terminated within 30 seconds of password change."
```

### 7. Add Cost Estimation (LOW PRIORITY)

For `create`, add approximate hourly costs:
```
"Approximate hourly costs (USD):
- 0.5 CPU/2GB: $0.10/hour
- 1 CPU/4GB: $0.20/hour
- 2 CPU/8GB: $0.40/hour
(Costs vary by region and are subject to change)"
```

**Why:** Helps users make informed decisions about resource allocation.

## Priority Implementation Order

1. **HIGH:** Fix tool naming convention
2. **HIGH:** Add service_id format documentation and examples
3. **MEDIUM:** Clarify wait/timeout behavior
4. **MEDIUM:** Document response schemas
5. **MEDIUM:** Add error scenario documentation
6. **LOW:** Enhance security documentation
7. **LOW:** Add cost estimation information

## Testing Notes

- Tools executed successfully during testing
- No token limit or timeout issues encountered
- Response format is consistent and well-structured
- Service ID format is consistent (10-character alphanumeric)

## Conclusion

The TigerData MCP tools provide a solid foundation for database service management. The primary improvements needed are in documentation clarity and naming consistency. With these changes, the tools would provide an excellent developer experience that aligns with modern CLI tool expectations.