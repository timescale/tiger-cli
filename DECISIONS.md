# Architecture Decision Records

This document records key architectural decisions made during the development of Tiger CLI.

## OpenAPI Client Generation - oapi-codegen

**Status**: Accepted  
**Date**: 2025-08-05  
**Decision**: Use oapi-codegen for generating Go client code from OpenAPI specifications

### Context

We needed to generate a Go client from our OpenAPI 3.0 specification (`openapi.yaml`) to enable type-safe API calls for features like API key validation during login. Several tools were evaluated:

1. **go-swagger**: Only supports Swagger 2.0, not OpenAPI 3.0
2. **openapi-generator-cli**: Java-based tool that generates traditional separation of concerns with separate model and API files, but requires Java Runtime and has more dependencies
3. **ogen**: Modern Go-native generator with excellent performance and OpenTelemetry support, but generates more complex code structure
4. **oapi-codegen**: Go-native tool that generates clean, simple, idiomatic Go code

### Decision

We chose **oapi-codegen** because:

- **Simplicity**: Generates the cleanest, most readable Go code with minimal boilerplate
- **Minimal dependencies**: Fewest external dependencies compared to alternatives
- **Idiomatic Go**: Output follows Go conventions and patterns naturally
- **Active maintenance**: Well-maintained with regular updates
- **Pure Go**: No external runtime dependencies (unlike openapi-generator-cli requiring Java)
- **Straightforward integration**: Easy to integrate into Go build processes

### Consequences

**Positive**:
- Clean, maintainable generated code that integrates well with existing Go codebase
- Minimal dependency footprint
- Fast generation without external runtime requirements
- Type-safe API client for all Tiger API operations

**Negative**:
- One more tool dependency for the project
- Generated code needs to be regenerated when OpenAPI spec changes

### Implementation

The generated client is located in `internal/tiger/oapi/` with:
- `client.go`: HTTP client implementation
- `types.go`: Type definitions for all API models

Generation command: `oapi-codegen -generate types,client -package oapi openapi.yaml`