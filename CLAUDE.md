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

# Load environment variables from .env file (note: source .env doesn't work)
export $(cat .env | xargs)

# Run integration tests with environment variables from .env file
export $(cat .env | xargs) && go test ./internal/tiger/cmd -v -run TestServiceLifecycleIntegration
```

### Running Locally
```bash
# After building, run the CLI
./bin/tiger --help

# Or run directly with go
go run ./cmd/tiger --help
```

### Integration Testing
```bash
# Run all tests (integration tests will skip without credentials)
go test ./...

# Run only integration tests
go test ./internal/tiger/cmd -run Integration

# To run integration tests with real API calls, set environment variables:
export TIGER_PUBLIC_KEY_INTEGRATION=your-public-key
export TIGER_SECRET_KEY_INTEGRATION=your-secret-key
export TIGER_PROJECT_ID_INTEGRATION=your-project-id

# Optional: Set this to test database commands with existing service
export TIGER_EXISTING_SERVICE_ID_INTEGRATION=existing-service-id

# Then run tests normally
go test ./internal/tiger/cmd -v -run Integration
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

## Development Best Practices

- Always use go fmt after making file changes and before committing

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

## Command Architecture: Pure Functional Builder Pattern

Tiger CLI uses a pure functional builder pattern with **zero global command state**. This architecture ensures perfect test isolation, eliminates shared state issues, and provides a clean, maintainable command structure.

### Philosophy

- **No global variables** - All commands, flags, and state are locally scoped
- **Functional builders** - Every command is built by a dedicated function 
- **Complete tree building** - `buildRootCmd()` constructs the entire CLI structure
- **Perfect test isolation** - Each test gets completely fresh command instances
- **Self-contained commands** - All dependencies passed explicitly via parameters

### Architecture Overview

```
buildRootCmd() → Complete CLI with all commands and flags
├── buildVersionCmd()
├── buildConfigCmd()
│   ├── buildConfigShowCmd()
│   ├── buildConfigSetCmd()
│   ├── buildConfigUnsetCmd()
│   └── buildConfigResetCmd()
├── buildAuthCmd()
│   ├── buildLoginCmd()
│   ├── buildLogoutCmd()  
│   └── buildWhoamiCmd()
├── buildServiceCmd()
│   ├── buildServiceListCmd()
│   ├── buildServiceDescribeCmd()
│   ├── buildServiceCreateCmd()
│   └── buildServiceUpdatePasswordCmd()
└── buildDbCmd()
    ├── buildDbConnectionStringCmd()
    ├── buildDbConnectCmd()
    └── buildDbTestConnectionCmd()
```

### Root Command Builder

The root command builder creates the complete CLI structure:

```go
func buildRootCmd() *cobra.Command {
    // Declare ALL flag variables locally within this function
    var cfgFile string
    var debug bool
    var output string
    var apiKey string
    var projectID string
    var serviceID string
    var analytics bool

    cmd := &cobra.Command{
        Use:   "tiger",
        Short: "Tiger CLI - TigerData Cloud Platform CLI",
        Long:  `Complete CLI description...`,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            // Use local flag variables in scope
            if err := logging.Init(debug); err != nil {
                return fmt.Errorf("failed to initialize logging: %w", err)
            }
            // ... rest of initialization
            return nil
        },
    }

    // Setup configuration and flags
    setupConfigAndFlags(cmd, &cfgFile, &debug, &output, &apiKey, &projectID, &serviceID, &analytics)
    
    // Add all subcommands (complete tree building)
    cmd.AddCommand(buildVersionCmd())
    cmd.AddCommand(buildConfigCmd())
    cmd.AddCommand(buildAuthCmd())
    cmd.AddCommand(buildServiceCmd())
    cmd.AddCommand(buildDbCmd())

    return cmd
}
```

### Simple Command Pattern

For commands without flags:

```go
func buildVersionCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "version",
        Short: "Show version information", 
        Long:  `Display version, build time, and git commit information.`,
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Printf("Tiger CLI %s\n", Version)
            // ... version output
        },
    }
}
```

### Commands with Local Flags

For commands that need their own flags:

```go
func buildMyFlaggedCmd() *cobra.Command {
    // Declare flag variables locally (NEVER globally!)
    var myFlag string
    var enableFeature bool
    var retryCount int
    
    cmd := &cobra.Command{
        Use:   "my-command",
        Short: "Command with local flags",
        RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) < 1 {
                return fmt.Errorf("argument required")
            }
            
            cmd.SilenceUsage = true
            
            // Use flag variables (they're in scope)
            fmt.Printf("Flag: %s, Feature: %t, Retries: %d\n", 
                myFlag, enableFeature, retryCount)
            return nil
        },
    }
    
    // Add flags - bound to local variables
    cmd.Flags().StringVar(&myFlag, "flag", "", "My flag description")  
    cmd.Flags().BoolVar(&enableFeature, "enable", false, "Enable feature")
    cmd.Flags().IntVar(&retryCount, "retries", 3, "Retry count")
    
    return cmd
}
```

### Parent Commands with Subcommands

For commands that contain subcommands, build the complete tree:

```go
func buildParentCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "parent",
        Short: "Parent command with subcommands",
        Long:  `Parent command containing multiple subcommands.`,
    }
    
    // Add all subcommands (builds complete subtree)
    cmd.AddCommand(buildChild1Cmd())
    cmd.AddCommand(buildChild2Cmd())
    cmd.AddCommand(buildChild3Cmd())
    
    return cmd
}
```

### Application Entry Point

The main application uses a single builder call:

```go
func Execute() {
    // Build complete command tree fresh each time
    rootCmd := buildRootCmd()
    
    err := rootCmd.Execute()
    if err != nil {
        if exitErr, ok := err.(interface{ ExitCode() int }); ok {
            os.Exit(exitErr.ExitCode())
        }
        os.Exit(1)
    }
}
```

### No init() Functions Needed

With this pattern, commands don't need init() functions because the root command builder handles complete tree construction:

```go
// OLD PATTERN (don't do this):
func init() {
    myCmd := buildMyCmd()
    rootCmd.AddCommand(myCmd)  // Global state dependency
}

// NEW PATTERN (do this):
// No init() function needed - buildRootCmd() handles everything
```

### Testing with Complete Architecture

Tests use the full root command builder:

```go
func executeCommand(args ...string) (string, error) {
    // Build complete CLI fresh for each test
    rootCmd := buildRootCmd()
    
    buf := new(bytes.Buffer)
    rootCmd.SetOut(buf)  
    rootCmd.SetErr(buf)
    rootCmd.SetArgs(args)
    
    err := rootCmd.Execute()
    return buf.String(), err
}

func TestMyCommand(t *testing.T) {
    // Each test gets completely fresh CLI instance
    output, err := executeCommand("my-command", "--flag", "value")
    
    if err != nil {
        t.Fatalf("Command failed: %v", err)
    }
    
    if !strings.Contains(output, "expected") {
        t.Errorf("Expected 'expected' in output: %s", output)
    }
}
```

### Advanced Testing: Flag Access

For tests that need to verify flag values:

```go
func executeAndReturnRoot(args ...string) (*cobra.Command, string, error) {
    rootCmd := buildRootCmd()
    
    buf := new(bytes.Buffer)
    rootCmd.SetOut(buf)
    rootCmd.SetArgs(args)
    
    err := rootCmd.Execute()
    return rootCmd, buf.String(), err
}

func TestFlagValues(t *testing.T) {
    rootCmd, output, err := executeAndReturnRoot("service", "create", "--name", "test")
    
    // Navigate to specific command
    serviceCmd, _, _ := rootCmd.Find([]string{"service"})
    createCmd, _, _ := serviceCmd.Find([]string{"create"})
    
    // Check flag value
    nameFlag := createCmd.Flags().Lookup("name")
    if nameFlag.Value.String() != "test" {
        t.Errorf("Expected name=test, got %s", nameFlag.Value.String())
    }
}
```

### Benefits of This Architecture

1. **Zero Global State**: No shared variables between commands or tests
2. **Perfect Test Isolation**: Each test builds completely fresh command trees  
3. **Simplified Initialization**: Single entry point builds everything
4. **Maintainable Code**: No complex global variable management
5. **Easy Development**: Add new commands by creating builders and adding to root
6. **Predictable Behavior**: No hidden dependencies or initialization order issues
7. **Memory Efficient**: Commands built only when needed

### Migration Guide

When adding new commands to this architecture:

1. **Create a builder function** following the `buildXXXCmd()` pattern
2. **Declare flags locally** within the builder function scope
3. **Add to root command** by calling `cmd.AddCommand(buildXXXCmd())` in `buildRootCmd()`
4. **No init() function** required - everything goes through the root builder
5. **Test with `buildRootCmd()`** instead of recreating flag setup

This architecture ensures Tiger CLI remains maintainable and testable as it grows.

## CLI Design Patterns and Conventions

Tiger CLI follows established command-line interface patterns, particularly inspired by the GitHub CLI (`gh`) for consistency with modern CLI tools.

### Boolean Flag Patterns

When implementing boolean flags that can be enabled or disabled, follow the GitHub CLI pattern:

1. **Default Positive Behavior** - The positive behavior is the default (no flag needed)
2. **Explicit Negative Override** - Use `--no-<feature>` to disable the default behavior
3. **Avoid Mutually Exclusive Pairs** - Don't create both `--enable-X` and `--disable-X` flags

**Example:**
```go
// ✅ Good: GitHub CLI pattern
var createNoSetDefault bool
cmd.Flags().BoolVar(&createNoSetDefault, "no-set-default", false, "Don't set this service as the default service")

// Default behavior: set as default
if !createNoSetDefault {
    setAsDefault()
}

// ❌ Avoid: Mutually exclusive flags
var setDefault bool
var noSetDefault bool
cmd.Flags().BoolVar(&setDefault, "set-default", true, "Set service as default")
cmd.Flags().BoolVar(&noSetDefault, "no-set-default", false, "Don't set as default")
cmd.MarkFlagsMutuallyExclusive("set-default", "no-set-default")
```

**Real GitHub CLI Examples:**
- `gh pr create` has `--no-maintainer-edit` to disable the default maintainer edit behavior
- Default behavior is implicit, override is explicit with `--no-` prefix

### Destructive Operation Patterns

For destructive operations (delete, remove, etc.), follow these safety patterns:

1. **Explicit Resource ID Required** - No default fallback for destructive operations
2. **Interactive Confirmation** - Require typing the resource ID to confirm
3. **Automation Override** - `--confirm` flag to skip prompts for scripts
4. **AI Agent Warnings** - Include warnings in help text for AI agents

**Example:**
```go
// Require explicit service ID (no default)
if len(args) < 1 {
    return fmt.Errorf("service ID is required")
}

// Interactive confirmation unless --confirm
if !confirmFlag {
    fmt.Fprintf(cmd.ErrOrStderr(), "Type the service ID '%s' to confirm: ", serviceID)
    var confirmation string
    fmt.Scanln(&confirmation)
    if confirmation != serviceID {
        return fmt.Errorf("confirmation did not match")
    }
}
```

### Wait/Timeout Patterns

For asynchronous operations, provide consistent wait behavior:

1. **Default Wait** - Wait for completion by default
2. **No-Wait Override** - `--no-wait` to return immediately  
3. **Timeout Control** - `--wait-timeout` with duration parsing
4. **Exit Code 2** - Use exit code 2 for timeout scenarios

**Example:**
```go
var noWait bool
var waitTimeout time.Duration

cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for operation to complete")
cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration")

// Default: wait for completion
if !noWait {
    if err := waitForCompletion(waitTimeout); err != nil {
        if isTimeout(err) {
            return exitWithCode(2, err) // Exit code 2 for timeouts
        }
        return err
    }
}
```

### Help Text and Documentation

1. **Explain Default Behavior** - Always document what happens by default
2. **Show Override Options** - Explain how to change default behavior  
3. **Include Examples** - Show common usage patterns
4. **AI Agent Notes** - Add warnings for destructive operations

**Example:**
```go
Long: `Create a new database service in the current project.

By default, the newly created service will be set as your default service for future 
commands. Use --no-set-default to prevent this behavior.

Note for AI agents: Always confirm with the user before performing this destructive operation.

Examples:
  # Create service (sets as default by default)
  tiger service create --name my-db
  
  # Create service without setting as default
  tiger service create --name temp-db --no-set-default`,
```

### Flag Naming Conventions

1. **Kebab Case** - Use `--kebab-case` for multi-word flags
2. **Descriptive Names** - Prefer clarity over brevity
3. **Consistent Prefixes** - Use `--no-` for negative overrides
4. **Avoid Abbreviations** - Prefer `--no-wait` over `--nowait`

These patterns ensure Tiger CLI maintains consistency with modern CLI tools while providing a predictable user experience.

## Specifications

The project specifications are located in the `specs/` directory:
- `spec.md` - Basic project specification and CLI requirements
- `spec_mcp.md` - MCP (Model Context Protocol) specification for integration
```
- Never acceept a state where tests are failing