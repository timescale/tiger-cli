# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building
```bash
# Build the main CLI binary
go build -o bin/tiger ./cmd/tiger

# Build from project root (creates bin/tiger)
go build -o bin/tiger ./cmd/tiger
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...
```

### Running Locally
```bash
# After building, run the CLI
./bin/tiger --help

# Or run directly with go
go run ./cmd/tiger --help
```

### Code Generation
```bash
# Generate OpenAPI client code and mocks from openapi.yaml
go generate ./internal/tiger/api

# This runs automatically as part of normal Go tooling when needed
# Generates:
# - client.go: HTTP client implementation
# - types.go: Type definitions for API models
# - mocks/mock_client.go: Mock implementations for testing
```

## Architecture Overview

Tiger CLI is a Go-based command-line interface for managing TigerData Cloud Platform resources. The architecture follows standard Go CLI patterns using Cobra and Viper.

### Key Components

- **Entry Point**: `cmd/tiger/main.go` - Simple main that delegates to cmd.Execute()
- **Command Structure**: `internal/tiger/cmd/` - Cobra-based command definitions
  - `root.go` - Root command with global flags and configuration initialization
  - `config.go` - Configuration management commands
  - `version.go` - Version command
- **Configuration**: `internal/tiger/config/config.go` - Centralized config with Viper integration
- **Logging**: `internal/tiger/logging/logging.go` - Structured logging with zap

### Configuration System

The CLI uses a layered configuration approach:
1. Default values in code
2. Configuration file at `~/.config/tiger/config.yaml`
3. Environment variables with `TIGER_` prefix
4. Command-line flags (highest precedence)

Key configuration values:
- `api_url`: TigerData API endpoint
- `project_id`: Default project ID
- `service_id`: Default service ID  
- `output`: Output format (json, yaml, table)
- `analytics`: Usage analytics toggle

### Logging Architecture

Two-mode logging system using zap:
- **Production mode**: Minimal output, warn level and above, clean formatting
- **Debug mode**: Full development logging with colors and debug level

Global flags available on all commands:
- `--debug`: Enable debug logging
- `--output/-o`: Set output format
- `--api-key`: Override API key
- `--project-id`: Override project ID
- `--service-id`: Override service ID
- `--analytics`: Toggle analytics

### Dependencies

- **Cobra**: CLI framework and command structure
- **Viper**: Configuration management with multiple sources
- **Zap**: Structured logging
- **oapi-codegen**: OpenAPI client generation (build-time dependency)
- **gomock**: Mock generation for testing (build-time dependency)
- **Go 1.23+**: Required Go version

## Project Structure

```
cmd/tiger/              # CLI entry point
internal/tiger/         # Internal packages
  cmd/                  # Cobra command definitions
  config/               # Configuration management
  logging/              # Structured logging utilities
  api/                  # Generated OpenAPI client (oapi-codegen)
    mocks/              # Generated mocks for testing
specs/                  # CLI specifications and API documentation
  spec.md               # Basic project specification
  spec_mcp.md           # MCP (Model Context Protocol) specification
bin/                    # Built binaries (created during build)
openapi.yaml            # OpenAPI 3.0 specification for TigerData API
tools.go                # Build-time dependencies
```

The `internal/` directory follows Go conventions to prevent external imports of internal packages.

## Cobra Usage Display Pattern

When implementing Cobra commands, use this pattern to control when usage information is displayed on errors:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    // 1. Do argument validation first - errors here show usage
    if len(args) < 1 {
        return fmt.Errorf("service ID is required")
    }
    
    // 2. Set SilenceUsage = true after argument validation
    cmd.SilenceUsage = true
    
    // 3. Proceed with business logic - errors here don't show usage
    if err := someAPICall(); err != nil {
        return fmt.Errorf("operation failed: %w", err)
    }
    
    return nil
},
```

**Philosophy**: 
- Early argument/syntax errors → show usage (helps users learn command syntax)
- Operational errors after arguments are validated → don't show usage (avoids cluttering output with irrelevant usage info)

This provides fine-grained control over when usage is displayed, improving user experience by showing help when it's relevant and hiding it when it's not.

## Command Builder Pattern

Tiger CLI uses a functional builder pattern for creating Cobra commands that ensures proper test isolation and eliminates global state issues. This pattern should be followed for all new commands.

### Philosophy

- **No global command variables** - Commands are built fresh each time
- **Local flag variables** - Flag variables are scoped within builder functions
- **Perfect test isolation** - Each test gets completely fresh command instances
- **Functional approach** - Builder functions return complete command trees

### Basic Pattern

For simple commands without flags:

```go
func buildMyCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "my-command",
        Short: "Description of my command",
        Long:  `Detailed description...`,
        RunE: func(cmd *cobra.Command, args []string) error {
            // Command logic here
            return nil
        },
    }
    
    return cmd
}
```

### Commands with Flags

For commands that need flags, declare flag variables locally within the builder:

```go
func buildMyFlaggedCmd() *cobra.Command {
    // Declare flag variables locally (not globally!)
    var myFlag string
    var myBoolFlag bool
    var myIntFlag int
    
    cmd := &cobra.Command{
        Use:   "my-command",
        Short: "Command with flags",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Use flag variables directly (they're in scope)
            fmt.Printf("Flag value: %s\n", myFlag)
            return nil
        },
    }
    
    // Add flags after command definition
    cmd.Flags().StringVar(&myFlag, "my-flag", "", "Description of my flag")
    cmd.Flags().BoolVar(&myBoolFlag, "my-bool", false, "Boolean flag")
    cmd.Flags().IntVar(&myIntFlag, "my-int", 0, "Integer flag")
    
    // Bind flags to viper for environment variable support
    viper.BindPFlag("my_flag", cmd.Flags().Lookup("my-flag"))
    
    return cmd
}
```

### Parent Commands with Subcommands

For commands that contain subcommands, build the entire tree in one function:

```go
func buildParentCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "parent",
        Short: "Parent command with subcommands",
        Long:  `Parent command that contains multiple subcommands.`,
    }
    
    // Add all subcommands
    cmd.AddCommand(buildChildCmd1())
    cmd.AddCommand(buildChildCmd2())
    cmd.AddCommand(buildChildCmd3())
    
    return cmd
}
```

### Integration in init()

The init() function should be minimal and use local variables:

```go
func init() {
    // Build command tree (no global variables needed)
    myCmd := buildMyCmd()
    
    // Add to parent command
    rootCmd.AddCommand(myCmd)
}
```

### Testing Commands with Builder Pattern

When testing commands built with this pattern:

```go
func executeTestCommand(args ...string) (string, error, *cobra.Command) {
    // Build fresh command tree for testing
    testCmd := buildMyCmd()
    
    // Create test root
    testRoot := &cobra.Command{Use: "test"}
    testRoot.AddCommand(testCmd)
    
    // Execute and return root for flag access
    buf := new(bytes.Buffer)
    testRoot.SetOut(buf)
    testRoot.SetArgs(args)
    
    err := testRoot.Execute()
    return buf.String(), err, testRoot
}

func TestMyCommand(t *testing.T) {
    output, err, rootCmd := executeTestCommand("my-command", "--my-flag", "value")
    
    // Navigate to specific command to check flags if needed
    myCmd, _, err := rootCmd.Find([]string{"my-command"})
    if err != nil {
        t.Fatalf("Failed to find command: %v", err)
    }
    
    // Access flag values through cobra's flag system
    flagValue := myCmd.Flags().Lookup("my-flag").Value.String()
    // Test assertions...
}
```

### Benefits of This Pattern

1. **Perfect Test Isolation**: Each test gets completely fresh commands with no shared state
2. **No Global State**: Eliminates issues with flag variables persisting between tests  
3. **Clean Architecture**: Commands are self-contained and easier to understand
4. **Easy Testing**: Can access any part of the command tree for verification
5. **Maintainable**: No complex reset logic or global variable management needed

### Example: Service Command Implementation

The service command demonstrates this pattern in action. See `buildServiceCmd()` in `internal/tiger/cmd/service.go` for a complete example of a parent command with multiple subcommands, each with their own flags.

## Specifications

The project specifications are located in the `specs/` directory:
- `spec.md` - Basic project specification and CLI requirements
- `spec_mcp.md` - MCP (Model Context Protocol) specification for integration