# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Naming Guidelines

When writing or updating documentation, code comments, CLI output, error messages, or any other user-facing text, use these official naming conventions:

### Official Names

- **Company Name**: "Tiger Data" (two words, with space)
- **Cloud Platform**: "Tiger Cloud" (NOT "TigerData Cloud" or "TigerData Cloud Platform")
- **CLI Tool**: "Tiger CLI"
- **MCP Server**: "Tiger MCP"
- **API References**: "Tiger Cloud API" (NOT "TigerData API")

### Examples

✅ **Correct Usage:**
- "Tiger CLI is a command-line interface for managing Tiger Cloud platform resources."
- "Authenticate with Tiger Cloud API"
- "List all database services in your Tiger Cloud project."
- "The Tiger MCP server provides programmatic access to Tiger Cloud."

❌ **Incorrect Usage:**
- ~~"TigerData Cloud Platform"~~ → Use "Tiger Cloud platform"
- ~~"TigerData API"~~ → Use "Tiger Cloud API"
- ~~"TigerData project"~~ → Use "Tiger Cloud project"
- ~~"TigerData MCP"~~ → Use "Tiger MCP"

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

#### Using the Test Script (Recommended)
```bash
# Run all integration tests (default pattern: Integration)
./scripts/test-integration.sh

# Run with verbose output
./scripts/test-integration.sh -v

# Run specific test pattern (overrides default)
./scripts/test-integration.sh -run CreateRole

# Run with custom timeout
./scripts/test-integration.sh -timeout 10m

# Combine flags (any go test flags are supported)
./scripts/test-integration.sh -v -run CreateRole_WithInheritedGrants -timeout 5m
```

The script automatically:
- Loads environment variables from `.env` file
- Builds the tiger CLI binary
- Runs tests matching "Integration" pattern by default
- Passes all arguments through to `go test` (supports all standard go test flags)

#### Manual Testing
```bash
# Run all tests (integration tests will skip without credentials)
go test ./...

# Run only integration tests
go test ./internal/tiger/cmd -run Integration

# To run integration tests with real API calls, set environment variables:
export TIGER_PUBLIC_KEY_INTEGRATION=your-public-key
export TIGER_SECRET_KEY_INTEGRATION=your-secret-key
export TIGER_API_URL_INTEGRATION=http://localhost:8080/public/api/v1

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

### Code Formatting and Validation

- Always use `go fmt` after making file changes and before committing
- Run `go vet ./...` to catch potential issues before committing
- Run `go test ./...` to ensure all tests pass

### Configuration Management

**IMPORTANT:** Follow these rules when working with configuration:

1. **Always use the Config struct** - Never read configuration values directly from the global viper instance. Always load a `Config` struct and use its fields.

2. **Load once, pass down** - Load the config once at the start of a command or operation, then pass it down to functions that need it. Do not reload the config if one is already available higher in the call chain.

3. **MCP tools reload per-call** - In MCP tool implementations, always load a fresh config at the start of each tool call. This ensures that configuration changes made by the user (via `tiger config set`) take effect immediately for the next tool call, without requiring the MCP server to be restarted.

**Example:**
```go
// ✅ Good: Load config once and pass it down
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
    // Load fresh config at start of MCP tool call
    cfg, err := s.loadConfigWithProjectID()
    if err != nil {
        return nil, ServiceListOutput{}, err
    }

    // Use cfg.ProjectID, cfg.ServiceID, etc.
    return doWork(cfg)
}

// ❌ Bad: Reading from viper directly
func handleCommand() {
    projectID := viper.GetString("project_id") // Don't do this
}

// ❌ Bad: Reloading config when already available
func processData(cfg *config.Config) {
    freshCfg, _ := config.Load() // Don't reload if cfg is already available
    // Use cfg instead
}
```

### CLI and MCP Synchronization

When implementing or updating functionality:

1. **Keep CLI commands and MCP tools in sync** - When updating a CLI command, check if there's a corresponding MCP tool and apply the same changes to keep them aligned. Examples:
   - `tiger service list` command → `service_list` MCP tool
   - `tiger service create` command → `service_create` MCP tool

2. **Check for intentional differences** - Some discrepancies between CLI and MCP are intentional (e.g., different default behaviors, different output formats). Before making changes to sync them, ask whether the difference is intentional. Document intentional differences in code comments.

3. **Share code between CLI and MCP** - Code that needs to be used by both CLI commands and MCP tools should be moved to a shared package (not in `internal/tiger/cmd` or `internal/tiger/mcp`). Current examples:
   - `internal/tiger/common/` - Shared business logic, password storage, wait operations, error handling, and other utilities that have dependencies on config/api packages
   - `internal/tiger/util/` - Small utility functions with minimal dependencies (formatting, validation, etc.)
   - `internal/tiger/api/` - API client used by both

### Documentation Synchronization

After making changes to commands, tools, configuration, or flags, always check and update:

- **README.md** - User-facing documentation (installation, usage, configuration)
- **CLAUDE.md** - Developer guidance (this file)
- **specs/spec.md** - CLI specification and requirements
- **specs/spec_mcp.md** - MCP server specification and tool documentation

Keep these files in sync with the actual implementation. When adding a new flag, config option, or command, update all relevant documentation files.

### Analytics Tracking

Tiger CLI tracks usage analytics to help improve the product. Analytics are automatically tracked using middleware - you typically don't need to add tracking code manually when adding new commands or MCP tools.

#### Automatic Tracking via Middleware

**CLI Commands** - All commands are automatically wrapped with analytics middleware in `internal/tiger/cmd/root.go:134`

This middleware:
- Automatically tracks all CLI commands with event name like `"Run tiger service create"`
- Captures elapsed time for each command
- Tracks all user-provided flags (excluding sensitive ones like passwords and keys)
- Records success/failure status and error messages
- Uses `analytics.TryInit()` to gracefully handle cases where credentials aren't available

**MCP Tools** - All MCP tool calls are automatically tracked via middleware in `internal/tiger/mcp/server.go:102`

This middleware:
- Automatically tracks all MCP tool calls with event name like `"Call service_create tool"`
- Extracts and tracks tool arguments (excluding sensitive fields like passwords, queries, parameters)
- Records success/failure status and error messages
- Also tracks resource reads and prompt requests

#### Event Naming Conventions

The middleware follows these automatic naming conventions:

**CLI Commands:** `"Run tiger <command> <subcommand>"`
- Example: `"Run tiger service create"`, `"Run tiger db connection-string"`

**MCP Tools:** `"Call <tool_name> tool"`
- Example: `"Call service_create tool"`, `"Call db_execute_query tool"`

**MCP Resources:** `"Read proxied resource"`
- Includes the `resource_uri` property

**MCP Prompts:** `"Get <prompt_name> prompt"`
- Example: `"Get setup_hypertable prompt"`, `"Get migrate_to_hypertables prompt"`

#### Excluding Sensitive Data

The middleware automatically excludes sensitive fields using a centralized ignore list in `internal/tiger/analytics/analytics.go`.

**Current ignore list:**
- `password` - User passwords
- `new_password` - New passwords for updates
- `public_key` - API public keys
- `secret_key` - API secret keys
- `project_id` - Project identifiers
- `query` - SQL queries (may contain sensitive data)
- `parameters` - SQL parameters (may contain sensitive data)

**IMPORTANT:** When adding new commands or MCP tools, review whether they introduce new sensitive flags, input parameters, or positional arguments:

1. **For sensitive flags or MCP tool parameters:** Add the field name to the `ignore` list in `internal/tiger/analytics/analytics.go`
   - Note: Flag names with dashes (like `public-key`) should be added with underscores (`public_key`) to the ignore list

2. **For positional arguments:** Currently, all positional arguments are tracked automatically. If a command is added that accepts sensitive data as a positional argument (not as a flag), you must either:
   - Refactor to use a flag instead
   - Add filtering logic in `wrapCommandsWithAnalytics()` in `internal/tiger/cmd/root.go` to sanitize or omit the args from tracking

**Common sensitive fields to watch for:**
- Credentials: API keys, tokens, passwords, secret keys
- User data: SQL queries, connection strings, personal information
- Security-related: Private keys, certificates, encryption keys

## Architecture Overview

Tiger CLI is a Go-based command-line interface for managing Tiger, the modern database cloud. The architecture follows standard Go CLI patterns using Cobra and Viper.

### Key Components

- **Entry Point**: `cmd/tiger/main.go` - Simple main that delegates to cmd.Execute()
- **Command Structure**: `internal/tiger/cmd/` - Cobra-based command definitions
  - `root.go` - Root command with global flags and configuration initialization
  - `auth.go` - Authentication commands (login, logout, status)
  - `service.go` - Service management commands (list, create, get, fork, start, stop, delete, update-password, logs)
  - `db.go` - Database operation commands (connection-string, connect, test-connection)
  - `config.go` - Configuration management commands (show, set, unset, reset)
  - `mcp.go` - MCP server commands (install, start, list, get)
  - `version.go` - Version command
- **Configuration**: `internal/tiger/config/config.go` - Centralized config with Viper integration
- **Logging**: `internal/tiger/logging/logging.go` - Structured logging with zap
- **API Client**: `internal/tiger/api/` - Generated OpenAPI client with mocks
- **MCP Server**: `internal/tiger/mcp/` - Model Context Protocol server implementation
  - `server.go` - MCP server initialization, tool registration, and lifecycle management
  - `service_tools.go` - Service management tools (list, get, create, fork, start, stop, update-password, logs)
  - `db_tools.go` - Database operation tools (execute-query)
  - `proxy.go` - Proxy client that forwards tools/resources/prompts from remote docs MCP server
  - `capabilities.go` - Lists all available MCP capabilities (tools, prompts, resources, resource templates)
- **Common Package**: `internal/tiger/common/` - Shared business logic used by both CLI and MCP
  - Password storage utilities (keyring, pgpass, validation)
  - Wait operations and polling logic (WaitForService)
  - Error handling and exit code utilities
  - Service detail conversion helpers
  - Log fetching with pagination (FetchServiceLogs)
- **Utilities**: `internal/tiger/util/` - Small utility functions with minimal dependencies

### Configuration System

The CLI uses a layered configuration approach:
1. Default values in code
2. Configuration file at `~/.config/tiger/config.yaml`
3. Environment variables with `TIGER_` prefix
4. Command-line flags (highest precedence)

For a complete list of valid configuration options, see `internal/tiger/config/config.go`.
All configuration options also have corresponding `TIGER_` environment variables.

For a complete list of global command-line flags, see `internal/tiger/cmd/root.go`.
Note that not all config options have corresponding global flags, and not all global flags correspond to config options.

### MCP Server Architecture

The Tiger MCP server provides AI assistants with programmatic access to Tiger resources through the Model Context Protocol (MCP).

**Two Types of Tools:**

1. **Direct Tiger Tools** - Native tools for Tiger operations
   - `service_tools.go` - Service management (list, get, create, fork, start, stop, update-password, logs)
   - `db_tools.go` - Database operations (execute-query)
2. **Proxied Documentation Tools** (`proxy.go`) - Tools forwarded from a remote docs MCP server (see `proxy.go` for implementation)

**Tool Definition Pattern:**

When defining MCP tools, we use a pattern that balances type safety with schema flexibility:

1. **Define input/output structs** with JSON tags:
```go
type ServiceCreateInput struct {
    Name      string   `json:"name,omitempty"`
    Region    string   `json:"region,omitempty"`
    Replicas  int      `json:"replicas,omitempty"`
    Wait      bool     `json:"wait,omitempty"`
}
```

2. **Implement Schema() method** that auto-generates base schema, then enhances it:
```go
func (ServiceCreateInput) Schema() *jsonschema.Schema {
    // Auto-generate schema from struct
    schema := util.Must(jsonschema.For[ServiceCreateInput](nil))

    // Add descriptions, examples, and validation
    schema.Properties["name"].Description = "Human-readable name for the service (auto-generated if not provided)"
    schema.Properties["name"].Examples = []any{"my-production-db", "analytics-service"}
    schema.Properties["name"].MaxLength = util.Ptr(128)

    schema.Properties["region"].Description = "AWS region where the service will be deployed"
    schema.Properties["region"].Examples = []any{"us-east-1", "us-west-2"}

    // Set defaults and constraints
    schema.Properties["replicas"].Default = util.Must(json.Marshal(0))
    schema.Properties["replicas"].Minimum = util.Ptr(0.0)
    schema.Properties["replicas"].Maximum = util.Ptr(5.0)

    // Define enums for constrained values
    schema.Properties["cpu_memory"].Enum = util.AnySlice(cpuMemoryCombinations)

    return schema
}
```

3. **Register tool with enhanced schema**:
```go
mcp.AddTool(s.mcpServer, &mcp.Tool{
    Name:        "service_create",
    Description: `Detailed multi-line description...`,
    InputSchema: ServiceCreateInput{}.Schema(),  // Uses our enhanced schema
}, s.handleServiceCreate)
```

**Key Benefits of This Pattern:**
- **Type safety**: Schema automatically reflects struct fields
- **Flexibility**: Can add descriptions, examples, validation, enums after generation
- **Maintainability**: Struct changes automatically propagate to schema
- **Rich documentation**: AI assistants get detailed guidance on each field
- **Fail-fast validation**: If you try to access/modify a property that doesn't exist in the generated schema (e.g., typo in field name), the code will panic at runtime, ensuring the schema stays in sync with the struct
- **LLM validation**: Stricter JSON schema validations (min/max values, enums, patterns, etc.) prevent LLMs from sending invalid arguments in tool calls, catching errors before they reach the handler

**Important Notes:**
- Fields with `omitempty` are optional; fields without it are required
- Always provide descriptions and examples for better AI assistant understanding
- Use JSON Schema properties to constrain and document values (e.g., `Default`, `Minimum`, `Maximum`, `Enum`, `Pattern`, `MinLength`, etc.)
  - See the [jsonschema-go Schema type](https://pkg.go.dev/github.com/google/jsonschema-go/jsonschema#Schema) for all available properties
- The MCP SDK can infer schemas automatically, but explicit schemas provide better control

### Logging Architecture

Two-mode logging system using zap:
- **Production mode**: Minimal output, warn level and above, clean formatting
- **Debug mode**: Full development logging with colors and debug level

### Dependencies

- **Cobra**: CLI framework and command structure
- **Viper**: Configuration management with multiple sources
- **Zap**: Structured logging
- **oapi-codegen**: OpenAPI client generation (build-time dependency)
- **gomock**: Mock generation for testing (build-time dependency)
- **go-sdk (MCP)**: Model Context Protocol SDK for AI assistant integration
- **pgx/v5**: PostgreSQL driver for database operations in MCP tools
- **Go 1.25+**: Required Go version

## Project Structure

```
tiger-cli/
├── cmd/tiger/              # Main CLI entry point
├── internal/tiger/         # Internal packages
│   ├── api/                # Generated OpenAPI client (oapi-codegen)
│   │   └── mocks/          # Generated mocks for testing
│   ├── config/             # Configuration management
│   ├── logging/            # Structured logging utilities
│   ├── mcp/                # MCP server implementation
│   ├── common/             # Shared business logic (password storage, wait ops, error handling, log fetching)
│   ├── cmd/                # CLI commands (Cobra)
│   └── util/               # Small utility functions with minimal dependencies
├── docs/                   # Documentation
│   └── development.md      # Development guide (building, testing, contributing)
├── specs/                  # CLI specifications and API documentation
│   ├── spec.md             # Basic project specification
│   └── spec_mcp.md         # MCP server specification
├── .github/workflows/      # GitHub Actions CI/CD
│   ├── test.yml            # Test workflow (runs on PRs and main)
│   └── release.yml         # Release workflow (runs on semver tags)
├── bin/                    # Built binaries (created during build)
├── openapi.yaml            # OpenAPI 3.0 specification for Tiger API
├── .goreleaser.yml         # GoReleaser configuration for building releases
├── tools.go                # Build-time dependencies
├── README.md               # User-facing documentation
└── CLAUDE.md               # Developer guidance for Claude Code
```

The `internal/` directory follows Go conventions to prevent external imports of internal packages.

**Additional Documentation:**
- See `docs/development.md` for detailed development information including building from source, running integration tests, and project structure details
- See `README.md` for user-facing documentation on installation, usage, and configuration

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
│   └── buildStatusCmd()
├── buildServiceCmd()
│   ├── buildServiceListCmd()
│   ├── buildServiceGetCmd()
│   ├── buildServiceCreateCmd()
│   ├── buildServiceForkCmd()
│   ├── buildServiceStartCmd()
│   ├── buildServiceStopCmd()
│   ├── buildServiceDeleteCmd()
│   ├── buildServiceUpdatePasswordCmd()
│   └── buildServiceLogsCmd()
├── buildDbCmd()
│   ├── buildDbConnectionStringCmd()
│   ├── buildDbConnectCmd()
│   └── buildDbTestConnectionCmd()
└── buildMCPCmd()
    ├── buildMCPInstallCmd()
    └── buildMCPStartCmd()
        ├── buildMCPStdioCmd()
        └── buildMCPHTTPCmd()
```

### Root Command Builder

The root command builder creates the complete CLI structure:

```go
func buildRootCmd() *cobra.Command {
    // Declare ALL flag variables locally within this function
    var configDir string
    var debug bool
    // ... other flag variables

    cmd := &cobra.Command{
        Use:   "tiger",
        Short: "Tiger CLI - Tiger Cloud Platform command-line interface",
        Long:  `Complete CLI description...`,
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            // Bind persistent flags to viper at execution time
            if err := errors.Join(
                viper.BindPFlag("debug", cmd.Flags().Lookup("debug")),
                // ... bind remaining flags
            ); err != nil {
                return fmt.Errorf("failed to bind flags: %w", err)
            }

            // Setup configuration and initialize logging
            // ... rest of initialization
        },
    }

    // Set up persistent flags
    cmd.PersistentFlags().StringVar(&configDir, "config-dir", config.GetDefaultConfigDir(), "config directory")
    cmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
    // ... add remaining persistent flags

    // Add all subcommands (complete tree building)
    cmd.AddCommand(buildVersionCmd())
    cmd.AddCommand(buildConfigCmd())
    // ... add remaining subcommands

    return cmd
}
```

See `internal/tiger/cmd/root.go` for the complete implementation.

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

### Commands with Flags That Need Viper Binding

For commands that need their flags bound to viper for configuration precedence (flag > env > config > default), use the `bindFlags()` helper:

```go
func buildMyConfigurableFlagCmd() *cobra.Command {
    var output string

    cmd := &cobra.Command{
        Use:     "my-command",
        Short:   "Command with configurable flag",
        PreRunE: bindFlags("output"),  // Binds flag to viper
        RunE: func(cmd *cobra.Command, args []string) error {
            cmd.SilenceUsage = true
            cfg, err := config.Load()  // Now includes bound flag value
            // ... use cfg.Output which respects: flag > env > config > default
        },
    }

    cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "output format")
    return cmd
}
```

The `bindFlags()` helper (defined in `internal/tiger/cmd/flag.go`) automatically converts flag names to config keys (e.g., `"new-password"` → `"new_password"`) and supports binding multiple flags: `bindFlags("output", "new-password")`.

**Why bind flags in PreRunE?**

Flags must be bound to viper at **execution time**, not at **build time**, for two critical reasons:

1. **Prevents binding conflicts**: When all commands are built at startup (the builder pattern), binding flags at build time can cause commands' flags to bind to the same viper keys, silently overwriting each other. Only the last binding wins.

2. **Ensures correct precedence**: Viper must bind flags after the command tree is built but before `config.Load()` is called. This happens in `PreRunE` (or `PersistentPreRunE` for persistent flags), ensuring the precedence order works correctly: command-line flags > environment variables > config file > defaults.

**Note:** Use `PreRunE` for command-specific flags, and `PersistentPreRunE` for persistent flags on the root command that apply to all subcommands.

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
3. **Bind flags to viper in PreRunE** if the flag needs to be configurable via config file or environment variables
4. **Add to root command** by calling `cmd.AddCommand(buildXXXCmd())` in `buildRootCmd()`
5. **No init() function** required - everything goes through the root builder
6. **Test with `buildRootCmd()`** instead of recreating flag setup

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

## GitHub Workflows

The project uses GitHub Actions for continuous integration and release automation. Workflows are defined in `.github/workflows/`:

### Test Workflow (`test.yml`)

**Trigger:** Runs on pull requests and pushes to `main` branch

**Purpose:** Validates code quality and ensures all tests pass

**Note:** Tests run with `-p 1` to avoid parallel execution issues with keyring access.

### Release Workflow (`release.yml`)

**Trigger:** Runs when a semver tag (e.g., `v1.2.3`) is pushed to the repository

**Purpose:** Builds and publishes releases across multiple platforms

**How to Trigger:**
```bash
# Method 1: Via GitHub UI (recommended)
# Go to Releases → Draft a new release → Create tag (v1.2.3) → Publish

# Method 2: Via command line
VERSION=1.2.3 && git tag -a v${VERSION} -m "${VERSION}" && git push origin v${VERSION}
```

**Publishing Targets:**
1. **GitHub Releases** - Creates release with binaries for multiple platforms (macOS, Linux, Windows)
2. **Homebrew Tap** - Updates `timescale/homebrew-tap` with new formula
3. **S3 Bucket** - Uploads binaries to `tiger-cli-releases` S3 bucket (behind `https://cli.tigerdata.com` CloudFront CDN) for install script and Homebrew downloads
4. **PackageCloud** - Publishes Debian (.deb) and RPM packages to `timescale/tiger-cli` repository

**Build Tool:** Uses [GoReleaser](https://goreleaser.com) to build and publish across all platforms. Configuration is in `.goreleaser.yml`.

## Specifications

The project specifications are located in the `specs/` directory:
- `spec.md` - Basic project specification and CLI requirements
- `spec_mcp.md` - MCP (Model Context Protocol) specification for integration

## Testing Guidelines

- Never accept a state where tests are failing
