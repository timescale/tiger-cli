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
export $(cat .env | xargs) && go test ./internal/cmd -v -run TestServiceLifecycleIntegration
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
go test ./internal/cmd -run Integration

# To run integration tests with real API calls, set environment variables:
export TIGER_PUBLIC_KEY_INTEGRATION=your-public-key
export TIGER_SECRET_KEY_INTEGRATION=your-secret-key
export TIGER_API_URL_INTEGRATION=http://localhost:8080/public/api/v1

# Optional: Set this to test database commands with existing service
export TIGER_EXISTING_SERVICE_ID_INTEGRATION=existing-service-id

# Optional: Set this to run the upgrade test against the live release CDN
export TIGER_UPGRADE_INTEGRATION=1

# Then run tests normally
go test ./internal/cmd -v -run Integration
```

### Code Generation
```bash
# Generate OpenAPI client code and mocks from openapi.yaml
go generate ./internal/api

# This runs automatically as part of normal Go tooling when needed
# Generates:
# - client.go: HTTP client implementation
# - types.go: Type definitions for API models
# - mocks/mock_client.go: Mock implementations for testing
```

Generation is driven by `internal/api/types.yaml` and `internal/api/client.yaml`
rather than command-line flags. Both set:

- `name-normalizer: ToCamelCaseWithInitialisms` — generated names capitalize
  initialisms the way Go does (`ServiceID`, not `ServiceId`; `CPUMillis`, not
  `CpuMillis`)
- `always-prefix-enum-values: true` — enum constants are prefixed with their type
  (`api.DeployStatusREADY`, not `api.READY`), so values from different enums can't
  collide in the package namespace

Keep these two configs in sync with each other, and with ghost's equivalents.
Changing either option renames identifiers across the whole codebase.

## Development Best Practices

### Code Formatting and Validation

- Always use `go fmt` after making file changes and before committing
- Run `go vet ./...` to catch potential issues before committing
- Run `go fix -diff ./...` to check for code that should be updated to use newer
  Go APIs or idioms
- Run `go tool staticcheck ./...` to catch additional issues (unused code, deprecated
  APIs, style checks) before committing — it's declared as a build-time tool in
  `go.mod`'s `tool (...)` block alongside `oapi-codegen` and `mockgen`
- Run `go test ./...` to ensure all tests pass

### Error Messages

Error strings start with a **lowercase** letter, so they read correctly when a
caller wraps them: `fmt.Errorf("failed to get service: %w", err)`. Log messages
are the opposite — they start with a capital letter (see "Logging Architecture").

The exception is a leading proper noun or initialism, which keeps its
capitalization (`fmt.Errorf("API key validation failed: %w", err)`). If that
reads awkwardly, reword so the identifier isn't first — `missing required option:
ClientName` rather than `ClientName is required`.

### Configuration Management

**IMPORTANT:** Follow these rules when working with configuration:

1. **There is no global config** - `config.Load(flags)` builds a fresh `viper` instance per call and unmarshals it into a `Config`. Nothing reads the global viper instance, and nothing should: always take a `*config.Config` and use its fields.

2. **Read the config from the App, don't load it yourself** - `common.App` (`internal/common/app.go`) holds the config and the API client built from it. `wrapCommands` loads it once per CLI invocation (and the MCP analytics middleware once per request); command bodies and helpers then read it with `app.GetAll()`, `app.GetConfig()`, or `app.GetClient()`. Call `config.Load` directly only where there is no App: `config.LoadForOutput` for `tiger config show`, and tests.

3. **The App owns flag precedence** - `app.SetFlags(cmd.Flags())` runs before `app.Load`, and `config.Load` binds the flags in `flagBindings` (`internal/config/config.go`) so precedence stays flag > env > file > default. `cmd.Flags()` includes the persistent flags inherited from parents, and flags a command doesn't define are skipped, so command-local flags (e.g. `--output`) bind only where they exist.

4. **Pass what you read down the call chain** - Pass the `*config.Config` (plus client and project ID where needed) to the functions that need them. Don't reload, and don't pass the App into `internal/common` helpers — they take the values they use, so they stay usable from both CLI and MCP.

5. **MCP reloads per request** - The analytics middleware in `internal/mcp/server.go` calls `s.app.Load(ctx)` on every request, so configuration changes (via `tiger config set`) and logins/logouts take effect on the next tool call without restarting the server. Tool handlers then read that state via `s.app.GetAll()`/`GetClient()` — they must not load it again.

**Example:**
```go
// ✅ Good: an MCP handler reads what the middleware loaded for this request
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
    client, projectID, err := s.app.GetClient()
    if err != nil {
        return nil, ServiceListOutput{}, err
    }

    return doWork(ctx, client, projectID)
}

// ✅ Good: a CLI command reads what wrapCommands loaded for this invocation
func run(cmd *cobra.Command, args []string) error {
    cfg, client, projectID, err := app.GetAll()
    // ...
}

// ❌ Bad: Reading from viper directly
func handleCommand() {
    projectID := viper.GetString("project_id") // Don't do this
}

// ❌ Bad: Loading again when the App already holds it — this also drops flag
// precedence, so --config-dir/--service-id are ignored
func run(cmd *cobra.Command, args []string) error {
    cfg, err := config.Load(nil) // Don't do this in a command
}

// ❌ Bad: Reloading config when already available
func processData(cfg *config.Config) {
    freshCfg, _ := config.Load(nil) // Don't reload if cfg is already available
    // Use cfg instead
}
```

Config-derived state lives on the `Config` too: credential storage is a set of
methods on it (`cfg.StoreCredentials`, `cfg.GetStoredCredentials`,
`cfg.RemoveCredentials`), keyed off `cfg.ConfigDir`, and `cfg.Set`/`Unset`/`Reset`
write the config file and then reload the struct **in place**. Reloading in place
is what lets a command change the config mid-run and have the App's readers see
it — that's how `tiger config set analytics false` suppresses its own analytics
event, and how `version_check false` suppresses the update notice.

### CLI and MCP Synchronization

When implementing or updating functionality:

1. **Keep CLI commands and MCP tools in sync** - When updating a CLI command, check if there's a corresponding MCP tool and apply the same changes to keep them aligned. Examples:
   - `tiger service list` command → `service_list` MCP tool
   - `tiger service create` command → `service_create` MCP tool

2. **Check for intentional differences** - Some discrepancies between CLI and MCP are intentional (e.g., different default behaviors, different output formats). Before making changes to sync them, ask whether the difference is intentional. Document intentional differences in code comments.

3. **Share code between CLI and MCP** - Code that needs to be used by both CLI commands and MCP tools should be moved to a shared package (not in `internal/cmd` or `internal/mcp`). Current examples:
   - `internal/common/` - Shared business logic, password storage, wait operations, error handling, and other utilities that have dependencies on config/api packages
   - `internal/util/` - Small utility functions with minimal dependencies (formatting, validation, etc.)
   - `internal/api/` - API client used by both

### Documentation Synchronization

After making changes to commands, tools, configuration, or flags, always check and update:

- **README.md** - User-facing documentation (installation, usage, configuration)
- **CLAUDE.md** - Developer guidance (this file)
- **docs/development.md** - Development guide (building, testing, contributing)

Keep these files in sync with the actual implementation. When adding a new flag, config option, or command, update all relevant documentation files.

### Analytics Tracking

Tiger CLI tracks usage analytics to help improve the product. Analytics are automatically tracked using middleware - you typically don't need to add tracking code manually when adding new commands or MCP tools.

#### Automatic Tracking via Middleware

**CLI Commands** - All commands are automatically wrapped with the per-invocation lifecycle in `wrapCommands()` in `internal/cmd/root.go`, which tracks analytics as one of its steps

This middleware:
- Automatically tracks all CLI commands with event name like `"Run tiger service create"`
- Captures elapsed time for each command
- Tracks all user-provided flags (excluding sensitive ones like passwords and keys)
- Records success/failure status and error messages
- Uses `analytics.TryInit()` to gracefully handle cases where credentials aren't available

**MCP Tools** - All MCP tool calls are automatically tracked via middleware in `analyticsMiddleware()` in `internal/mcp/server.go`

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

The middleware automatically excludes sensitive fields using a centralized ignore list in `internal/analytics/analytics.go`.

**Current ignore list:**
- `password` - User passwords
- `new_password` - New passwords for updates
- `public_key` - API public keys
- `secret_key` - API secret keys
- `project_id` - Project identifiers
- `query` - SQL queries (may contain sensitive data)
- `parameters` - SQL parameters (may contain sensitive data)

**IMPORTANT:** When adding new commands or MCP tools, review whether they introduce new sensitive flags, input parameters, or positional arguments:

1. **For sensitive flags or MCP tool parameters:** Add the field name to the `ignore` list in `internal/analytics/analytics.go`
   - Note: Flag names with dashes (like `public-key`) should be added with underscores (`public_key`) to the ignore list

2. **For positional arguments:** Currently, all positional arguments are tracked automatically. If a command is added that accepts sensitive data as a positional argument (not as a flag), you must either:
   - Refactor to use a flag instead
   - Add filtering logic in `wrapCommands()` in `internal/cmd/root.go` to sanitize or omit the args from tracking

**Common sensitive fields to watch for:**
- Credentials: API keys, tokens, passwords, secret keys
- User data: SQL queries, connection strings, personal information
- Security-related: Private keys, certificates, encryption keys

## Architecture Overview

Tiger CLI is a Go-based command-line interface for managing Tiger, the modern database cloud. The architecture follows standard Go CLI patterns using Cobra and Viper.

### Key Components

- **Entry Point**: `cmd/tiger/main.go` - Simple main that delegates to cmd.Execute()
- **Command Structure**: `internal/cmd/` - Cobra-based command definitions for all
  CLI commands (auth, service, db, config, mcp, version, upgrade, completion).
  Each command lives in its own file, named to match the command in snake_case
  (see "One File Per Command" below). `root.go` holds the root command, global
  flags, and configuration initialization. Files ending in `_helper.go` hold
  cross-group helpers rather than commands — see "Where Helpers Go" below.
  - `db_connect.go` - The whole `db connect`/`psql` flow, including read replica selection: in an interactive terminal, when the service has one or more active read replicas (listed via the `/replicaSets` API), prompts to connect to the primary or one of the replicas. Skipped when stdin is not a TTY, when `--no-replica-prompt` is set, or when the service has no read replicas. Also handles password recovery when the stored password is rejected.
  - `upgrade.go` - Self-update command (download latest release, verify checksum, replace running binary in place)
- **Configuration**: `internal/config/config.go` - `Config` struct plus load/write
  helpers. Each `Load` uses its own viper instance (no global state); see
  "Configuration Management" above
- **API Client**: `internal/api/` - Generated OpenAPI client with mocks
- **MCP Server**: `internal/mcp/` - Model Context Protocol server implementation.
  Each MCP tool lives in its own file, named to match the tool (see "One File Per
  MCP Tool" below). `server.go` holds server initialization, tool registration,
  and lifecycle management. Helper files hold shared utilities (`utils.go`,
  `errors.go`), the remote docs proxy (`proxy.go`), and capability listing
  (`capabilities.go`).
- **Common Package**: `internal/common/` - Shared business logic used by both CLI and MCP
  - `App` (`app.go`) - per-invocation config + API client, shared by CLI commands and MCP handlers
  - Password storage utilities (keyring, pgpass, validation)
  - Wait operations and polling logic (WaitForService)
  - Connection detail helpers (GetConnectionDetails, GetReplicaConnectionDetails for read replicas)
  - Error handling and exit code utilities
  - Service detail conversion helpers
  - Log fetching with pagination (FetchServiceLogs)
- **Utilities**: `internal/util/` - Small utility functions with minimal dependencies (formatting, validation, password generation)

### HTTP Requests

All outgoing HTTP requests should use the shared `api.HTTPClient` defined in
`internal/api/client_util.go`. This client has a built-in 30-second request
timeout and sets the CLI's User-Agent on every request. `NewTigerClient` and
`NewTigerClientWithToken` use it automatically. If you need to make HTTP
requests outside of the API client, use `api.HTTPClient` directly rather than
`http.DefaultClient` or creating a new `http.Client`.

If a request needs a shorter timeout, set one via the context:
```go
ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
defer cancel()
```

If a request needs a longer timeout, clone `api.HTTPClient` and override its
`Timeout` rather than building a new `http.Client` from scratch — this keeps
the User-Agent (see `upgradeHTTPClient` in `internal/cmd/upgrade.go`).

### Configuration System

The CLI uses a layered configuration approach:
1. Default values in code
2. Configuration file at `~/.config/tiger/config.yaml`
3. Environment variables with `TIGER_` prefix
4. Command-line flags (highest precedence)

For a complete list of valid configuration options, see `internal/config/config.go`.
All configuration options also have corresponding `TIGER_` environment variables.

For a complete list of global command-line flags, see `internal/cmd/root.go`.
Note that not all config options have corresponding global flags, and not all global flags correspond to config options.

### Experimental Feature Gating

Some surfaces (currently `tiger service metrics …` CLI commands and the
`service_metrics_*` MCP tools) call gateway endpoints marked
`x-tigerdata-preview: true` in `openapi.yaml` — their request/response shape is
still in flux. Upstream (`savannah-gateway/internal/rest/openapi.yaml`) uses the
same marker; the Stainless SDK pipeline drops `x-tigerdata-preview` operations
entirely, but oapi-codegen ignores the extension, so tiger-cli's generated v1
client includes them. We gate access at registration time, behind an
intentionally-undocumented env var — `TIGER_EXPERIMENTAL` (default `false`).

**This is env-var only** — deliberately not a config-file key, not a flag, and
not surfaced by `tiger config show`. It mirrors ghost's `GHOST_EXPERIMENTAL`
pattern: `strconv.ParseBool(os.Getenv("TIGER_EXPERIMENTAL"))` is read once in
`buildRootCmd` and stored on the `App` as `app.Experimental`, which both the CLI
and the MCP server read at registration time.

- CLI: `buildServiceCmd` guards `cmd.AddCommand(buildServiceMetricsCmd(app))` with `if app.Experimental { … }`. When the env var is unset, the `metrics` subtree isn't added to the command tree at all — the command literally does not exist (no help entry, no tab completion, `unknown command` error like any typo).
- MCP: `NewServer` passes `app.Experimental` to `registerServiceTools(readOnly, experimental bool)`, which guards the metrics tool `addTool` calls the same way, so the tools aren't advertised to MCP clients when the env var is off. Restart the MCP server after toggling.

**Do not mention `TIGER_EXPERIMENTAL` in user-facing docs, command help, spec
files, or error messages.** When a feature graduates, remove the
`x-tigerdata-preview: true` marker upstream, delete the `if app.Experimental { … }` / `if experimental
{ … }` wrappers (both CLI and MCP), drop the `experimental bool` parameter from
`registerServiceTools`, and remove the `Experimental` field from `common.App`. The
call sites already use the normal v1 client — no client wiring needs to change.

### MCP Server Architecture

The Tiger MCP server provides AI assistants with programmatic access to Tiger resources through the Model Context Protocol (MCP).

**Two Types of Tools:**

1. **Direct Tiger Tools** - Native tools for Tiger service management and database operations, one file per tool
2. **Proxied Documentation Tools** (`proxy.go`) - Tools forwarded from a remote docs MCP server (see `proxy.go` for implementation)

**Server State:**

`NewServer(ctx, app, logger)` takes the already-loaded `*common.App` and keeps it
on the `Server`, along with the logger (nil is replaced with a discarding one;
see "Logging Architecture"). Read-only mode, the experimental gate, and the
docs-proxy settings are read once here at startup (a client must restart the
server to pick those up), while the analytics middleware calls `s.app.Load(ctx)`
on every request so tool handlers see current config and credentials. Handlers
therefore never load anything themselves — they read `s.app.GetAll()`,
`s.app.GetClient()`, or `s.app.GetConfig()`, and log via `s.logger`.

**One File Per MCP Tool:**

Every tool gets its own file in `internal/mcp/`, named to match the tool:
`service_create` → `service_create.go`, `db_execute_query` → `db_execute_query.go`.
Each tool file is laid out in this order:

1. The `<Tool>Input`/`<Tool>Output` structs and their `Schema()` methods
2. The `new<Tool>Tool()` function returning the `*mcp.Tool`
3. The `handle<Tool>` handler method on `*Server`
4. Helper functions used only by that tool

Registration lives in `server.go` (`registerServiceTools`, `registerDatabaseTools`),
so adding a tool means adding one file plus one `addTool` line. Shared schema
helpers and API-to-output conversion live in `utils.go`.

**Read-Only Mode Gate:**

Write/destructive MCP tool handlers and CLI command `RunE` functions must call `common.CheckReadOnly(cfg)` (defined in `internal/common/errors.go`) immediately after reading the config from the App. When `cfg.ReadOnly` is `true`, the call returns `common.ErrReadOnly` and the API client is never invoked. The gated CLI commands today are `service create`, `service fork`, `service start`, `service stop`, `service resize`, `service update-password`, and `service delete`.

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

3. **Define a `new*Tool()` constructor** returning the tool with its enhanced schema:
```go
func newServiceCreateTool() *mcp.Tool {
    return &mcp.Tool{
        Name:        "service_create",
        Description: `Detailed multi-line description...`,
        InputSchema: ServiceCreateInput{}.Schema(),  // Uses our enhanced schema
    }
}
```

4. **Register the tool** in `registerServiceTools`/`registerDatabaseTools` in `server.go`:
```go
addTool(s, readOnly, newServiceCreateTool(), s.handleServiceCreate)
```

**Key Benefits of This Pattern:**
- **Type safety**: Schema automatically reflects struct fields
- **Flexibility**: Can add descriptions, examples, validation, enums after generation
- **Maintainability**: Struct changes automatically propagate to schema
- **Rich documentation**: AI assistants get detailed guidance on each field
- **Fail-fast validation**: If you try to access/modify a property that doesn't exist in the generated schema (e.g., typo in field name), the code will panic at runtime, ensuring the schema stays in sync with the struct
- **LLM validation**: Stricter JSON schema validations (min/max values, enums, patterns, etc.) prevent LLMs from sending invalid arguments in tool calls, catching errors before they reach the handler

**Important Notes:**
- Fields with `omitempty` or `omitzero` are optional; fields without either are
  required. For a struct-typed (not pointer) optional field, use `omitzero`
  instead of `omitempty` — plain `encoding/json`'s `omitempty` has no effect on
  a struct-valued field (it's never the zero value in the way `omitempty`
  checks for), so the field would always be required in the schema despite the
  tag.
- Always provide descriptions and examples for better AI assistant understanding
- Use JSON Schema properties to constrain and document values (e.g., `Default`, `Minimum`, `Maximum`, `Enum`, `Pattern`, `MinLength`, etc.)
  - See the [jsonschema-go Schema type](https://pkg.go.dev/github.com/google/jsonschema-go/jsonschema#Schema) for all available properties
- The MCP SDK can infer schemas automatically, but explicit schemas provide better control

### Logging Architecture

Only the MCP server logs. `newLogger(w io.Writer)`
(`internal/cmd/logger_helper.go`) points the standard `log` package at `w` and
returns `slog.Default()`; `tiger mcp start` (stdio and http) calls it with
`cmd.ErrOrStderr()` and passes the result to `mcp.NewServer`, which uses it for
its own output and hands it to the MCP SDK (`mcp.ServerOptions.Logger` and the
docs proxy's `mcp.ClientOptions.Logger`) — so the SDK's own session-lifecycle
lines land on the same stream. `mcp.NewServer(ctx, app, nil)` — used by
`mcp list`, `mcp get`, and completion, which only enumerate capabilities —
discards the output via `slog.New(slog.DiscardHandler)`.

There is no level configuration and no `--debug` flag. `slog.Default()` drops
anything below `Info` unless `slog.SetLogLoggerLevel` is called, so log at `Info`
or above; a `Debug` call would silently go nowhere. Attach errors with
`slog.Any("error", err)`, not `slog.String("error", err.Error())`. Log messages
start with a capital letter — error strings do the opposite (see "Error
Messages").

Because every statement is visible by default, keep them sparse: log failures
that would otherwise be swallowed (the docs-proxy registration errors are the
model), not per-step tracing of work that succeeded. A default `tiger mcp start`
is silent; the startup lines that do exist report configuration that removes
capabilities — the docs proxy being disabled, and each write tool skipped in
read-only mode — so a client can see why a tool it expected is missing.

Everything outside `internal/mcp` writes to stdout/stderr directly rather than
logging. Don't add log statements to CLI commands — print with `cmd.Print*` /
`cmd.PrintErr*` (see "Output Streams"), or return an error.

### Dependencies

- **Cobra**: CLI framework and command structure
- **Viper**: Configuration management with multiple sources
- **slog**: Structured logging for the MCP server
- **BubbleTea v2**: Interactive terminal menus and spinners (`charm.land/bubbletea/v2`)
- **oapi-codegen**: OpenAPI client generation (build-time dependency)
- **gomock**: Mock generation for testing (build-time dependency)
- **go-sdk (MCP)**: Model Context Protocol SDK for AI assistant integration
- **pgx/v5**: PostgreSQL driver for database operations in MCP tools
- **Go 1.27+**: Required Go version

## Project Structure

```
tiger-cli/
├── cmd/tiger/              # Main CLI entry point
├── internal/               # Internal packages
│   ├── analytics/          # Usage analytics tracking
│   ├── api/                # Generated OpenAPI client (oapi-codegen)
│   │   └── mocks/          # Generated mocks for testing
│   ├── config/             # Configuration management
│   ├── mcp/                # MCP server implementation (one file per tool)
│   ├── common/             # Shared business logic (password storage, wait ops, error handling, log fetching)
│   ├── cmd/                # CLI commands (Cobra, one file per command)
│   ├── util/               # Small utility functions with minimal dependencies
│   └── version/            # Version check / update notification
├── docs/                   # Documentation
│   └── development.md      # Development guide (building, testing, contributing)
├── .github/workflows/      # GitHub Actions CI/CD
│   ├── test.yml            # Test workflow (runs on PRs and main)
│   └── release.yml         # Release workflow (runs on semver tags)
├── bin/                    # Built binaries (created during build)
├── openapi.yaml            # OpenAPI 3.0 specification for Tiger API
├── .goreleaser.yaml        # GoReleaser configuration for building releases
├── tools.go                # Build-time dependencies
├── README.md               # User-facing documentation
└── CLAUDE.md               # Developer guidance for Claude Code
```

The `internal/` directory follows Go conventions to prevent external imports of internal packages.

**Additional Documentation:**
- See `docs/development.md` for detailed development information including building from source, running integration tests, and project structure details
- See `README.md` for user-facing documentation on installation, usage, and configuration

## Cobra Usage Display Pattern

Every leaf command (one with a `RunE`) sets `SilenceUsage: true` as a literal
field on its `cobra.Command` struct, unconditionally. A bad flag or a bad
argument count therefore prints only the error, not the full help text:

```go
cmd := &cobra.Command{
    Use:               "get [service-id]",
    Args:              cobra.MaximumNArgs(1),
    ValidArgsFunction: serviceIDCompletion(app),
    SilenceUsage:      true,
    RunE: func(cmd *cobra.Command, args []string) error {
        // ...
    },
}
```

**Philosophy**: usage output is only useful the first time someone learns a
command's syntax, and by then `--help` (or the docs) has already shown it. A
wrong flag or a failed API call both just need the one-line error; burying it
under a wall of flag descriptions makes it harder to read, not easier.

Because `SilenceUsage` lives on the struct literal, it's already `true` before
`RunE` ever runs — including for a flag-parsing error, which cobra reports
before calling `RunE` at all. There's nothing to set inside the handler.

Parent/group commands (`tiger service`, `tiger mcp`) have no `RunE` and don't
set it — cobra's own "unknown command" error for a bad subcommand name still
shows that command's usage, which is the one case where seeing the available
subcommands actually helps.

`SilenceErrors` is a separate setting and is rarely needed: set it only on a
command that already reports its own errors, so cobra doesn't print them a second
time. `tiger mcp start http` sets it because the MCP server logs failures through
slog before returning them.

## Command Architecture: Pure Functional Builder Pattern

Tiger CLI uses a pure functional builder pattern with **zero global command state**. This architecture ensures perfect test isolation, eliminates shared state issues, and provides a clean, maintainable command structure.

### Philosophy

- **No global variables** - All commands, flags, and state are locally scoped
- **Functional builders** - Every command is built by a dedicated function
- **Complete tree building** - `buildRootCmd()` constructs the entire CLI structure
- **Perfect test isolation** - Each test gets completely fresh command instances
- **Self-contained commands** - All dependencies passed explicitly via parameters

### Architecture Overview

`buildRootCmd(ctx)` builds the whole tree: it creates the `*common.App` that
carries per-invocation state, adds a `build*Cmd(app)` per top-level command, and
each group command adds its own subcommand builders. To see the current tree,
read `buildRootCmd()` in `root.go` and follow the builders down.

Every builder takes the `*common.App` and passes it to its children, so any
command body can reach the config and API client without loading them itself. The
App is what makes "load once per invocation" possible while keeping commands
free of global state.

### Per-Invocation Lifecycle

There are no `PersistentPreRunE`/`PersistentPostRunE` hooks. `wrapCommands()`
wraps the `RunE` of every command in the tree with the shared lifecycle, in this
order:

1. `app.SetFlags(cmd.Flags())` then `app.Load(ctx)` — the single config + API
   client load for the invocation
2. `color.NoColor` from `cfg.Color`
3. `versionCheck(...)` — starts a background release check, deferring the print
   so it lands after the command's own output
4. analytics — deferred, so it records the command's outcome and re-reads the App
   (see "Configuration Management")

Commands cobra adds after `wrapCommands` runs — `help`, `completion`, and the
`__complete` command behind tab completion — are deliberately **not** wrapped, so
they never touch the config file, the system keyring, or the network. Group
commands (`tiger service`) have no `RunE` and are skipped for the same reason.
Completion functions that do need the config or client wrap themselves with
`withAppLoad` (`completion_helper.go`).

### One File Per Command

Every command gets its own file in `internal/cmd/`, named to match the command in
snake_case: `tiger service create` → `service_create.go`, `tiger db create role`
→ `db_create_role.go`. Group commands with no `RunE` of their own still get a
file (`tiger service` → `service.go`).

Within a command's file:

1. Constants and package-level variables (if any) come first
2. The `build*Cmd()` function comes next
3. Helper functions used only by that command follow it

### Where Helpers Go

Place a helper by who calls it, working down this list until one matches:

1. **One command** → that command's file.
2. **Several commands in one group** → the group file (`service.go`, `db.go`).
3. **Across groups** → a package-level `<topic>_helper.go` file:
   `completion_helper.go`, `flag_helper.go`, `logger_helper.go`,
   `password_helper.go`.
4. **A genuine standalone utility** — small and isolated, with no notion of a
   command (`util.GenerateSecurePassword`) → `internal/util`. Anything shaped
   around the CLI stays in `cmd` even if its signature looks generic.
5. **Used by both CLI and MCP** → `internal/common`.

The `_helper.go` suffix is reserved for rule 3, so every other file in
`internal/cmd` is named after a command and contains a `build*Cmd()`.

Shell completion functions are an exception to rule 1: they all live in
`completion_helper.go`, however many commands use them.

Apply rule 1 even when the helper is large. `db_connect.go` holds the whole
`db connect` flow — argument splitting, read replica selection, password
recovery, and the psql handoff, bubbletea models and all — because nothing else
calls into it. `auth_login.go` likewise holds the entire OAuth flow. A long file
whose contents all serve one command is easier to follow than several short files
with entry points scattered across them.

Tests mirror this layout: `service_create.go` → `service_create_test.go`.
Package-wide test scaffolding (`TestMain`, auth mocks, shared command runners)
lives in `main_test.go`; per-group test helpers live in the group's test file.
`integration_test.go` stays a single cross-command suite.

### Root Command Builder

The root command builder creates the complete CLI structure:

```go
func buildRootCmd(ctx context.Context) (*cobra.Command, error) {
    // Per-invocation state, threaded through every builder
    app := &common.App{Experimental: experimental}

    cmd := &cobra.Command{
        Use:   "tiger",
        Short: "Tiger CLI - Tiger Cloud Platform command-line interface",
        Long:  `Complete CLI description...`,
    }

    // Cobra copies this onto the command it executes
    cmd.SetContext(ctx)

    // Set up persistent flags
    cmd.PersistentFlags().String("config-dir", config.GetDefaultConfigDir(), "config directory")
    skipUpdateCheck := cmd.PersistentFlags().Bool("skip-update-check", false, "skip checking for updates on startup")
    // ... add remaining persistent flags

    // Add all subcommands (complete tree building)
    cmd.AddCommand(buildVersionCmd(app))
    cmd.AddCommand(buildConfigCmd(app))
    // ... add remaining subcommands

    // Wrap every RunE in the tree with the shared lifecycle
    wrapCommands(cmd, app, skipUpdateCheck)

    return cmd, nil
}
```

See `internal/cmd/root.go` for the complete implementation.

### Simple Command Pattern

For commands without flags:

```go
func buildVersionCmd(app *common.App) *cobra.Command {
    return &cobra.Command{
        Use:   "version",
        Short: "Show version information",
        Long:  `Display version, build time, and git commit information.`,
        RunE: func(cmd *cobra.Command, args []string) error {
            fmt.Printf("Tiger CLI %s\n", Version)
            // ... version output
            return nil
        },
    }
}
```

Use `RunE`, not `Run`: only `RunE` commands are wrapped by `wrapCommands`, so a
`Run` command would silently skip the config load, analytics, and version check.

### Commands with Local Flags

For commands that need their own flags:

```go
func buildMyFlaggedCmd(app *common.App) *cobra.Command {
    // Declare flag variables locally (NEVER globally!)
    var myFlag string
    var enableFeature bool
    var retryCount int

    cmd := &cobra.Command{
        Use:          "my-command",
        Short:        "Command with local flags",
        SilenceUsage: true,
        RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) < 1 {
                return fmt.Errorf("argument required")
            }

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

### Commands with Flags That Override Config Values

A flag that should override a config value needs no wiring in the command: the
lifecycle wrapper already hands the command's flag set to `config.Load`, and the
binding table in `internal/config/config.go` does the rest.

Don't bind such a flag to a variable — use `String`/`Bool`/`Var` rather than
`StringVar`/`BoolVar`/`VarP(&x, …)`. The command must read the value from the
config, and a variable in scope is an invitation to read the raw flag instead,
which silently bypasses the env var and config file. `tiger version` had exactly
that bug. Flag types that validate at parse time (`outputFlag` and friends in
`flag_helper.go`) still work: register them with `new(outputFlag)`.

```go
func buildMyConfigurableFlagCmd(app *common.App) *cobra.Command {
    cmd := &cobra.Command{
        Use:          "my-command",
        Short:        "Command with configurable flag",
        SilenceUsage: true,
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg := app.GetConfig()
            // ... use cfg.Output which respects: flag > env > config > default
        },
    }

    cmd.Flags().VarP(new(outputFlag), "output", "o", "output format")
    return cmd
}
```

To make a *new* flag override a config value, add it to `flagBindings`, which
maps flag names to config keys (e.g. `"password-storage"` → `"password_storage"`).
Bindings are applied per load against the flag set passed in, so a flag bound for
one command never leaks into another, and a command that doesn't define the flag
simply skips it.

Flags that aren't config values (e.g. `--auto-generate`) stay as plain local
variables; a flag that only needs an env-var fallback can read it directly (see
`--new-password` and `TIGER_NEW_PASSWORD` in `service_update_password.go`).

### Parent Commands with Subcommands

For commands that contain subcommands, build the complete tree:

```go
func buildParentCmd(app *common.App) *cobra.Command {
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
func Execute(ctx context.Context) error {
    // Build complete command tree fresh each time
    rootCmd, err := buildRootCmd(ctx)
    if err != nil {
        return err
    }

    return rootCmd.Execute()
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
func executeCommand(ctx context.Context, args ...string) (string, error) {
    // Build complete CLI fresh for each test, including its App
    rootCmd, err := buildRootCmd(ctx)
    if err != nil {
        return "", err
    }

    buf := new(bytes.Buffer)
    rootCmd.SetOut(buf)
    rootCmd.SetErr(buf)
    rootCmd.SetArgs(args)

    err = rootCmd.Execute()
    return buf.String(), err
}

func TestMyCommand(t *testing.T) {
    // Each test gets completely fresh CLI instance
    output, err := executeCommand(t.Context(), "my-command", "--flag", "value")

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
func executeAndReturnRoot(ctx context.Context, args ...string) (*cobra.Command, string, error) {
    rootCmd, err := buildRootCmd(ctx)
    if err != nil {
        return nil, "", err
    }

    buf := new(bytes.Buffer)
    rootCmd.SetOut(buf)
    rootCmd.SetArgs(args)

    err = rootCmd.Execute()
    return rootCmd, buf.String(), err
}

func TestFlagValues(t *testing.T) {
    rootCmd, output, err := executeAndReturnRoot(t.Context(), "service", "create", "--name", "test")

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

1. **Create a builder function** following the `buildXXXCmd(app *common.App)` pattern
2. **Declare flags locally** within the builder function scope
3. **Use `RunE`** (not `Run`) so the command gets the shared lifecycle from `wrapCommands`
4. **Read config and client from the App** (`app.GetAll()`/`GetConfig()`/`GetClient()`) rather than loading them
5. **Add the flag to `flagBindings`** (in `internal/config/config.go`) if it should override a config value, and declare it without a variable so it can only be read back from the config
6. **Add to root command** by calling `cmd.AddCommand(buildXXXCmd(app))` in `buildRootCmd()`
7. **No init() function** required - everything goes through the root builder
8. **Test with `buildRootCmd(ctx)`** instead of recreating flag setup

This architecture ensures Tiger CLI remains maintainable and testable as it grows.

## CLI Design Patterns and Conventions

Tiger CLI follows established command-line interface patterns, particularly inspired by the GitHub CLI (`gh`) for consistency with modern CLI tools.

### Output Streams

`buildRootCmd` calls `cmd.SetOut(os.Stdout)` and `cmd.SetErr(os.Stderr)`. This is
required: cobra's `cmd.Print*` helpers write to `OutOrStderr()`, which falls back
to **stderr** when no out writer is set, so without it every `cmd.Printf` would
land on the wrong stream.

Commands print with the cobra helpers — `cmd.Print`/`Printf`/`Println` for
stdout, `cmd.PrintErr`/`PrintErrf`/`PrintErrln` for stderr. Don't use
`fmt.Print*` (bypasses the command's writers entirely, so tests can't capture it)
and don't spell out `fmt.Fprintf(cmd.OutOrStdout(), …)`. Reach for
`cmd.OutOrStdout()`/`cmd.ErrOrStderr()` only where an `io.Writer` is genuinely
required: `util.SerializeToJSON`/`SerializeToYAML`, `tablewriter.NewWriter`,
`tea.WithOutput`, and helpers in other packages (see below). Likewise use
`cmd.InOrStdin()` rather than `os.Stdin`.

What goes where:

1. **stdout** — the command's primary output: the data payload (table, JSON,
   YAML, env vars) and, in the plain-text path, the result text itself.
2. **stderr** — errors and warnings; interactive UI (confirmation prompts,
   password prompts, spinners); and progress/status messages that accompany
   structured output, so they never pollute a piped stream. `tiger service
   create` is the model: every status line is `cmd.PrintErrf` and only the final
   service payload goes to stdout.

Helper functions inside `internal/cmd` take the `*cobra.Command` and print
through it. The exception is a helper whose whole job is rendering into a writer
— `outputServiceTable`, `outputVersionTable`, `outputCapabilitiesTable` — which
keeps an `io.Writer` parameter because `tablewriter` needs one anyway.

Code in other packages (`internal/common`, `internal/version`) must not import
cobra. Those take an `io.Writer` (`common.WaitForServiceArgs.Output`,
`common.SpinnerArgs.Output`, `version.PrintUpdateWarning`) and the caller in
`internal/cmd` passes `cmd.ErrOrStderr()` or `cmd.OutOrStdout()`.

### Reading Stdin

Read through `cmd.InOrStdin()`, never `os.Stdin`, and use the helpers in
`internal/util/read.go` rather than driving `bufio`/`term` directly:

- `util.ReadLine(ctx, cmd.InOrStdin())` — one line, trimmed. Confirmation
  prompts and plain text input.
- `util.ReadPassword(ctx, cmd.InOrStdin())` — reads without echoing.
- `util.ReadAll(ctx, cmd.InOrStdin())` — everything, for piped input.

All three run the blocking read on a goroutine and select on `ctx.Done()`, so
Ctrl-C unblocks a waiting prompt instead of hanging until the user hits enter.
`ReadPassword` also saves and restores the terminal state, so a cancelled prompt
doesn't leave the shell in raw mode.

**Every read that prompts the user in real time must be gated on
`util.IsTerminal(cmd.InOrStdin()) && util.IsTerminal(cmd.ErrOrStderr())`**, and
must return an error naming the non-interactive alternative when either is false
— see `service update-password` ("use --new-password flag, --auto-generate flag,
or TIGER_NEW_PASSWORD environment variable") and `service delete` ("use --confirm
to skip the prompt"). Both streams have to be terminals: stdin so the user can
answer, stderr so they can see what they're being asked. A prompt nobody can see
reads as a hang, and can be answered by accident — a stray Enter at `service
update-password` rotates the password. Confirmations, password prompts, and
BubbleTea programs all follow this rule.

The gate is about *prompts*, not about reading stdin as such. A read whose whole
point is piped input — `util.ReadAll` on a here-doc or a `|` — must not be
gated, since a TTY is exactly what it doesn't expect.

Put the gate immediately before the prompt in the command body rather than
inside the shared helper, so the error can name the flag or env var that command
offers. `util.IsTerminal` takes `any`, so it works for both readers and writers.

`util.IsTerminal` and `util.ReadPassword` are `var`s so tests can replace them:
`stubIsTerminal(t, true)` and `stubReadPassword(t, "pw")` in `main_test.go` do
this with `t.Cleanup`. `ReadPassword` needs stdin to be a real `*os.File`, so a
password prompt can't be driven with `cmd.SetIn`; `ReadLine`/`ReadAll` take any
`io.Reader` and work fine with it.

### Interactive Terminal UIs

Interactive menus and spinners use BubbleTea (`charm.land/bubbletea/v2`). Always
pass all four of these to `tea.NewProgram`:

```go
program := tea.NewProgram(model,
    tea.WithInput(cmd.InOrStdin()),
    tea.WithOutput(cmd.ErrOrStderr()),
    tea.WithContext(cmd.Context()),
    tea.WithoutSignalHandler())
```

- `tea.WithInput` — **always**, even for a write-only UI like a spinner.
  BubbleTea asks the terminal for keyboard disambiguation (the Kitty keyboard
  protocol), so Ctrl+C arrives as a key press on stdin rather than as a SIGINT.
  It asks for that while rendering, no matter what it was given for input, so
  `tea.WithInput(nil)` doesn't bring the signal back — it just leaves the key
  press unread, and Ctrl+C does nothing at all. Easy to omit with no compile
  error: BubbleTea falls back to `os.Stdin`, and opens `/dev/tty` outright if
  that isn't a terminal.
- `tea.WithOutput` — **stderr**: a menu is interactive UI, not the command's
  result (see "Output Streams").
- `tea.WithContext` — so a SIGTERM caught by `main.go` unwinds the program. Skip
  it only when the model already has its own cancellation path that would race
  it, as `common.NewSpinner` does: `WaitForService`'s polling loop owns that
  shutdown and stops the spinner itself.
- `tea.WithoutSignalHandler` — Ctrl+C is handled as a key press and other
  signals reach `main.go`'s handler, so BubbleTea's own handler would only be a
  second owner of the same shutdown.

**Every model must handle `"ctrl+c"` itself** — nothing else will. A write-only
program turns it into a `context.CancelFunc` call so background goroutines unwind
cleanly; `spinnerModel` in `internal/common/spinner.go` is the model for that,
and it's what makes Ctrl+C work during a `--wait`.

Gate every program on both streams, as "Reading Stdin" describes.
`common.NewSpinner` follows the same rule, printing one message per line instead
when either stream isn't a terminal.

Because these helpers need both streams, they take the `*cobra.Command` rather
than an `io.Writer` — which is why `oauthLogin` holds one. Code in
`internal/common` can't import cobra, so `WaitForServiceArgs` and `SpinnerArgs`
carry the `Input`/`Output` fields the caller fills in instead.

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
    if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
        return fmt.Errorf("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
    }
    cmd.PrintErrf("Type the service ID '%s' to confirm: ", serviceID)
    confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
    if err != nil {
        return fmt.Errorf("failed to read confirmation: %w", err)
    }
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

### Case Insensitivity

Commands, aliases, and flags are matched case-insensitively (configured in `root.go`). Always declare them in lowercase — that's the canonical form. Two names that differ only in case are a collision and must be avoided.

These patterns ensure Tiger CLI maintains consistency with modern CLI tools while providing a predictable user experience.

## GitHub Workflows

The project uses GitHub Actions for continuous integration and release automation. Workflows are defined in `.github/workflows/`:

### Test Workflow (`test.yml`)

**Trigger:** Runs on pull requests and pushes to `main` branch

**Purpose:** Validates code quality and ensures all tests pass

**Note:** Tests never touch the system keyring — `TestMain` swaps in an in-memory mock (`keyring.MockInit()`), so CI needs no keyring setup.

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

**Build Tool:** Uses [GoReleaser](https://goreleaser.com) to build and publish across all platforms. Configuration is in `.goreleaser.yaml`.

## Testing Guidelines

- Never accept a state where tests are failing

### Unit Test Pattern

Command tests live in `internal/cmd`, one test file per command file, and all
follow a single table-driven pattern built on the shared harness in
`main_test.go`:

- `runCommand(t, args, setupMock, opts...)` builds the real root command via
  `buildRootCmd`, injects a generated mock API client
  (`internal/api/mocks.MockClientWithResponsesInterface`) through
  `app.SetClientFactory`, runs against an isolated `t.TempDir()` config
  directory (always passing `--analytics=false --skip-update-check`), and
  returns captured, ANSI-stripped stdout/stderr plus the Execute error.
- Test cases use the `cmdTest` struct and run through `runCmdTests`, which
  asserts `wantErr`, `wantStdout`, and `wantStderr` with **exact** matching
  (`assertOutput`, a go-cmp diff). When `wantErr` is set and `wantStderr`
  isn't, stderr is expected to be `"Error: <wantErr>\n"`. An optional `check`
  func holds extra assertions (config file contents, stored credentials, exit
  codes via `errors.As` on `common.ExitCodeError`).
- Options configure the run: `withStdin`, `withEnv`, `withConfig` (seed config
  file keys), `withConfigDir` (chain commands sharing one config dir),
  `withStoredCredentials`, `withClientError`/`withNotLoggedIn`,
  `withIsTerminal`, `withReadPassword`, `withOpenBrowser`, `withContext`.
- Mock expectations use `validCtx` for context arguments and exact request
  structs; `httpResponse(status)` and `sampleService(overrides...)` keep
  tables concise.
- `TestMain` calls `keyring.MockInit()` (tests never touch the system
  keyring; `internal/common` and `internal/config` do the same) and scrubs
  inherited `TIGER_*` env vars, preserving `TIGER_*_INTEGRATION`.
- Order cases by the command's execution flow: auth errors, argument/flag
  validation, read-only gate, network/API errors, nil response body, success
  paths (text, json, yaml), then remaining flags and edge cases.
- Test commands as a whole through `runCommand` — don't unit-test individual
  helpers unless their behavior is unreachable through a command.
- Commands that reach a real database (pgx) or spawn external binaries test
  their error paths only; the success paths are covered by
  `integration_test.go`.
