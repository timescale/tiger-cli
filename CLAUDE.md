# Tiger CLI

## Overview

Tiger CLI is a Go command-line interface for managing Tiger Cloud database services, built on Cobra and Viper. The same binary also runs Tiger MCP (`tiger mcp start`), a Model Context Protocol server that exposes service management and database operations as tools for AI assistants. CLI commands and MCP tools share their business logic through `internal/common`.

## Naming

Use the official product names in all user-facing text (documentation, code comments, CLI output, error messages):

- Company: **Tiger Data** (two words, with space)
- Cloud platform: **Tiger Cloud** (never "TigerData Cloud" or "TigerData Cloud Platform")
- CLI tool: **Tiger CLI**
- MCP server: **Tiger MCP**
- API: **Tiger Cloud API** (never "TigerData API")

## Repository Structure

- **`cmd/`** - Binary entry points. Contains `tiger/main.go`, the main CLI binary, which sets up signal handling (SIGINT/SIGTERM cancel the command context), delegates to `cmd.Execute()`, and maps a `common.ExitCodeError` found anywhere in the returned error chain to the process exit code.
- **`internal/`** - All core application logic (non-public Go packages).
  - **`internal/cmd/`** - Cobra command implementations for all CLI commands (auth, project, service, db, config, mcp, version, upgrade, completion). Each command lives in its own file, named to match the command in snake_case (see [Command Architecture](#command-architecture)). `root.go` holds the root command, global flags, and command tree construction; files ending in `_helper.go` contain cross-group helpers rather than commands.
  - **`internal/api/`** - API client layer. Includes the OpenAPI-generated REST client (`client.go`, `types.go`), the shared HTTP client (`client_util.go`), and a generated mock of the client interface for testing (`mocks/`). **Never edit the generated files by hand** — see [Code Generation](#code-generation).
  - **`internal/config/`** - Configuration management. Handles config file loading (via Viper), the `Config` struct and its write helpers, and credential storage (keyring with a file fallback in the config directory).
  - **`internal/common/`** - Shared business logic used by both CLI commands and MCP tools: the per-invocation `App` (config + API client), database password storage (keyring, pgpass), wait/polling operations and the spinner, connection detail helpers (including read replicas), service readiness checks, log fetching, and error handling with exit codes.
  - **`internal/mcp/`** - The Model Context Protocol (MCP) server. Each MCP tool lives in its own file, named to match the tool (see [MCP Server](#mcp-server)). `server.go` holds server initialization, tool registration, and lifecycle management; `proxy.go` forwards tools from a remote documentation MCP server; `capabilities.go` backs `mcp list`/`mcp get`.
  - **`internal/analytics/`** - Usage analytics tracking with sensitive-data redaction.
  - **`internal/util/`** - Small general-purpose utilities with minimal dependencies: formatting, validation, context-aware stdin reading, JSON/YAML serialization, terminal detection, password generation.
  - **`internal/version/`** - Version checking and update notifications.
- **`pkg/`** - Public Go packages. Contains `mcpinstall`, an API for installing MCP server configurations into AI coding assistants and editors, exposed as thin aliases over the `mcp install` logic in `internal/cmd`. Changes to install behavior are part of this package's public surface.
- **`docs/`** - Documentation. `development.md` is the development guide (building from source, testing, contributing).
- **`scripts/`** - Build and installation scripts (`install.sh`, `install.ps1`, completions generation) and the integration test runner (`test-integration.sh`).
- **`openapi.yaml`** - OpenAPI spec for the Tiger Cloud API, from which the API client is generated.
- **`.github/`** - GitHub Actions CI/CD workflows for testing (`test.yml`) and releases (`release.yml`).
- **`.goreleaser.yaml`** - GoReleaser configuration for building and publishing releases.
- **`Dockerfile`** - The container image published to `ghcr.io/timescale/tiger-cli`, which defaults to running `tiger mcp start`.
- **`docker-compose.yaml`** - Builds and runs that image locally.
- **`server-template.json`** - MCP registry manifest template. The release workflow substitutes the version into it to produce `server.json` and publishes that to the MCP registry.

## Build & Test

```bash
go install ./...   # build and install the tiger binary
go test ./...      # run all tests (integration tests skip without credentials)
go generate ./...  # regenerate the API client and mocks after editing openapi.yaml
```

You can also run without installing via `go run ./cmd/tiger --help`.

Before committing, run `go fmt ./...`, `go vet ./...`, `go fix -diff ./...`, `go tool staticcheck ./...` (staticcheck is declared as a build-time tool in `go.mod`'s `tool` block), and `go test ./...`.

Integration tests run via `./scripts/test-integration.sh [-v] [-run Pattern] [any go test flags]`, which loads environment variables from `.env`, builds the binary, and defaults to `-run Integration`. Credentials come from `TIGER_PUBLIC_KEY_INTEGRATION`, `TIGER_SECRET_KEY_INTEGRATION`, and `TIGER_API_URL_INTEGRATION`. Optionally, `TIGER_EXISTING_SERVICE_ID_INTEGRATION` tests the database commands against an existing service, and `TIGER_UPGRADE_INTEGRATION` runs the upgrade test against the live release CDN.

## Code Generation

The API client and mocks in `internal/api/` are generated from `openapi.yaml`. Never edit `client.go`, `types.go`, or `mocks/mock_client.go` by hand — edit the spec and run `go generate ./...` to regenerate everything. If the mock is stale, `go vet` will report that it no longer implements the client interface.

Generation is configured by `internal/api/types.yaml` and `internal/api/client.yaml` rather than command-line flags. Both set `name-normalizer: ToCamelCaseWithInitialisms` (generated names capitalize initialisms the Go way: `ServiceID`, not `ServiceId`) and `always-prefix-enum-values: true` (enum constants are prefixed with their type: `api.DeployStatusREADY`, so values from different enums can't collide). Keep the two configs in sync with each other — changing either option renames identifiers across the whole codebase.

## Command Architecture

The CLI has **zero global command state**: `buildRootCmd(ctx)` builds the entire command tree fresh on each invocation, and no command uses `init()` registration or package-level flag variables.

- `buildRootCmd` creates the per-invocation `*common.App` and passes it to a `build*Cmd(app)` builder function for each command; group builders add their own subcommand builders in turn. Any command body can reach the config and API client through the App without loading them itself.
- `wrapCommands` wraps the `RunE` of every command in the tree with the shared per-invocation lifecycle: `app.SetFlags(cmd.Flags())` followed by `app.Load(ctx)` (the single config + API client load), setting `color.NoColor` from `cfg.Color`, starting a background version check (whose output is deferred until after the command's own), and deferred analytics tracking. There are no `PersistentPreRunE`/`PersistentPostRunE` hooks.
- Always use `RunE`, never `Run` — a `Run` command would silently skip the lifecycle. Cobra's built-in `help`, `completion`, and `__complete` commands are added after `wrapCommands` runs and are deliberately left unwrapped, so they never touch the config file, the system keyring, or the network. Completion functions that do need the config or client wrap themselves with `withAppLoad` (`completion_helper.go`).
- Declare flags as local variables inside the builder function. Flags that override config values are the exception — see [Configuration](#configuration).

### One File Per Command

Every command gets its own file in `internal/cmd/`, named to match the command in snake_case: `tiger service create` → `service_create.go`. Group commands with no `RunE` of their own still get a file (`tiger service` → `service.go`). Within a file, constants and package-level variables come first, then the `build*Cmd()` function, then helpers used only by that command. Tests mirror this layout (`service_create.go` → `service_create_test.go`), with package-wide test scaffolding in `main_test.go` and `integration_test.go` as a single cross-command suite.

### Where Helpers Go

Place a helper by who calls it, working down this list until one matches:

1. **One command** → that command's file, even when the helper is large. `db_connect.go` holds the entire `db connect`/`psql` flow (read-replica selection, password recovery, the psql handoff) and `auth_login.go` the entire OAuth flow — a long file whose contents all serve one command is easier to follow than several short files with scattered entry points.
2. **Several commands in one group** → the group file (`service.go`, `db.go`).
3. **Across groups** → a package-level `<topic>_helper.go` file. The `_helper.go` suffix is reserved for this, so every other file in `internal/cmd` is named after a command.
4. **A genuine standalone utility** — small and isolated, with no notion of a command → `internal/util`. Anything shaped around the CLI stays in `cmd` even if its signature looks generic.
5. **Used by both CLI and MCP** → `internal/common`.

Exception: all shell completion functions — both `ValidArgsFunction` completions and flag-value completions — live in `completion_helper.go`, however many commands use them.

## Configuration

Configuration is layered, with precedence **flags > `TIGER_*` env vars > config file (`~/.config/tiger/config.yaml`) > defaults**. The complete list of options lives in `internal/config/config.go`, and the global flags in `internal/cmd/root.go` (the two don't correspond one-to-one).

- **There is no global config.** `config.Load` builds a fresh viper instance per call; nothing reads the global viper instance. Commands and MCP handlers read configuration through the App — `app.GetAll()`, `app.GetConfig()`, or `app.GetClient()` — and never call `config.Load` themselves. The only places that load directly are `config.LoadForOutput` (for `tiger config show`) and tests.
- **Pass what you read down the call chain.** Hand the `*config.Config` (plus client and project ID where needed) to the functions that need them rather than reloading. Don't pass the App into `internal/common` helpers — they take the specific values they use, which keeps them usable from both CLI and MCP.
- **The App owns flag precedence.** `config.Load` binds the flags listed in `flagBindings` (`internal/config/config.go`) against the flag set it's given, so a flag bound for one command never leaks into another, and a command that doesn't define a flag simply skips it.
- **Flags that override config values** must be added to `flagBindings` and declared *without* a bound variable (`cmd.Flags().String(...)`, or `Var(new(outputFlag), ...)` for validating flag types), so the only way to read the value is through the config. A variable in scope is an invitation to read the raw flag instead, silently bypassing the env var and config file. Flags that aren't config values stay plain local variables; one that only needs an env-var fallback can read the env var directly (see `--new-password` in `service_update_password.go`).
- **Config-derived state lives on the `Config` too.** Credential storage is a set of methods on it (`cfg.StoreCredentials`, `cfg.GetStoredCredentials`, `cfg.RemoveCredentials`), keyed off `cfg.ConfigDir`, and `cfg.Set`/`Unset`/`Reset` write the config file and then reload the struct **in place** — which is how a command's own config change (e.g. `tiger config set analytics false`) becomes visible to the rest of its invocation. `Set` also returns the value as stored, which isn't always its argument (`read_only` normalizes its legacy spellings) — echo the return value to the user, not the input.
- **The active project lives in the stored credentials, not the config file.** Switching projects (`tiger project use`) therefore requires an OAuth login — an API key is scoped to one project — and any project change clears a stale default `service_id` (see `clearStaleDefaultService` in `project_helper.go`, which `auth login` calls too).
- **MCP reloads per request.** The analytics middleware in `internal/mcp/server.go` calls `s.app.Load(ctx)` on every request, so config changes and logins/logouts take effect on the next tool call without restarting the server. Tool handlers read that state and must not load it again.

### Experimental Feature Gating

`TIGER_EXPERIMENTAL` gates commands and MCP tools that aren't ready to be public yet, for whatever reason — including, but not limited to, anything backed by a gateway endpoint marked `x-tigerdata-preview: true` in `openapi.yaml` (those request/response shapes are still in flux, so a surface built on one must always be gated).

It's an env var **only**: deliberately not a config key, not a flag, and hidden from `tiger config show`. `buildRootCmd` reads it once into `app.Experimental`, and the CLI guards its `AddCommand` calls with it while the MCP server guards its `addTool` calls, so when the env var is off the gated commands and tools don't exist at all — no help entry, no completion, not advertised to MCP clients (restart the MCP server after toggling). **Never mention `TIGER_EXPERIMENTAL` in user-facing docs, command help, or error messages.** When a feature graduates, delete the gates on both sides.

## Command Patterns

All cobra commands must:

1. Set `SilenceUsage: true` as a literal field on the `cobra.Command` struct of every leaf command (one with a `RunE`), so a bad flag or argument prints only the one-line error rather than a wall of usage text — and because it's on the struct literal, it's already set when cobra reports flag-parsing errors before `RunE` runs. Parent/group commands don't set it: their "unknown command" errors usefully show the available subcommands. `SilenceErrors` is separate and rarely needed — set it only on a command that already reports its own errors (`mcp start http` logs failures through slog).
2. Set `ValidArgsFunction` or `ValidArgs` for shell completions — `ValidArgsFunction` for dynamic completions (e.g. service IDs), `ValidArgs` for static lists, and `cobra.NoFileCompletions` when no completions apply. The important thing is that one of these is always set, so completion never falls back to filenames.
3. Use `RunE`, not `Run` (see [Command Architecture](#command-architecture)).
4. Print through the command — `cmd.Print*` for stdout, `cmd.PrintErr*` for stderr — and read via `cmd.InOrStdin()`. Never use `fmt.Print*` (it bypasses the command's writers, so tests can't capture it) or `os.Stdin`/`os.Stdout`/`os.Stderr` directly — the sole exception is `buildRootCmd` wiring stdout and stderr onto the root command (see [Streams](#streams)). Reach for `cmd.OutOrStdout()`/`cmd.ErrOrStderr()` only where an `io.Writer` is genuinely required (serializers, `tablewriter`, `tea.WithOutput`, helpers in other packages).
5. Pass `cmd.Context()` down into any long-running or cancellable operation, so commands exit promptly on Ctrl+C or SIGTERM.

Further conventions:

- Commands, aliases, and flags are matched **case-insensitively** (configured in `root.go`). Always declare them in lowercase — that's the canonical form — and never create two names differing only in case. Consider adding common aliases (`ls` for `list`, `rm` for `delete`) to match user expectations from other CLI tools.
- Flags use kebab-case, prefer clarity over brevity, and avoid abbreviations (`--no-wait`, not `--nowait`).
- **Boolean flags** follow the GitHub CLI pattern: the positive behavior is the default (no flag needed), and an explicit `--no-<feature>` flag disables it. Don't create mutually exclusive `--enable-X`/`--disable-X` pairs.
- **Destructive operations** (delete, password rotation) require an explicit service ID with no default fallback, prompt the user to type the resource ID to confirm, and offer a `--confirm` flag to skip the prompt for automation. The prompt must be TTY-gated (see [Reading Stdin](#reading-stdin)), and the help text should include a "Note for AI agents: always confirm with the user before performing this destructive operation" warning.
- **Long-running operations** wait for completion by default, with `--no-wait` to return immediately and `--wait-timeout` (a duration) to bound the wait; a timeout exits with code 2 (`common.ExitTimeout`). Use `common.WaitForService`, which shows a spinner on a TTY and plain progress lines otherwise, writing progress to stderr.
- **Help text** should document the default behavior, explain how to override it, and include examples of common usage.

## Output

### Streams

`buildRootCmd` calls `cmd.SetOut(os.Stdout)` and `cmd.SetErr(os.Stderr)`. Both are **required**: cobra's `cmd.Print*` helpers write to `OutOrStderr()`, which falls back to stderr when no out writer is set, so without them every `cmd.Printf` in the CLI would silently land on the wrong stream.

- **stdout** gets the command's primary output: the data payload (table, JSON, YAML, env vars), and in the plain-text path the result text itself.
- **stderr** gets everything else: errors and warnings; interactive UI (confirmation prompts, password prompts, spinners); and progress/status messages that accompany structured output, so a piped stdout stays clean. `tiger service create` is the model — every status line is `cmd.PrintErrf`, and only the final service payload goes to stdout.

Helper functions inside `internal/cmd` take the `*cobra.Command` and print through it. A helper whose whole job is rendering into a writer (the `output*Table` functions) takes an `io.Writer` instead, since `tablewriter` needs one anyway. Code in other packages (`internal/common`, `internal/version`) must not import cobra — it takes an `io.Writer` (e.g. `common.WaitForServiceArgs.Output`) and the caller in `internal/cmd` passes `cmd.ErrOrStderr()` or `cmd.OutOrStdout()`.

### Formatting

- Commands with structured output take `-o`/`--output` with `json`, `yaml`, and `table` (the default); some commands support extras (`env`, `bare`). Register the flag with the validating flag types in `flag_helper.go` (`new(outputFlag)` and friends, with no bound variable) and read the value from `cfg.Output` — `output` is a config value.
- Serialize with `util.SerializeToJSON` and `util.SerializeToYAML`. **Don't add `yaml:` struct tags to output types** — `SerializeToYAML` encodes to JSON first and converts, so only `json:` tags matter and the two formats stay consistent (including for generated types that only carry `json:` tags).
- Colored output uses `fatih/color`; the lifecycle sets `color.NoColor` from `cfg.Color`, so commands need no per-command wiring.

### Reading Stdin

Read user input through the helpers in `internal/util/read.go` — `util.ReadLine` (one trimmed line), `util.ReadPassword` (no echo, saves and restores terminal state), and `util.ReadAll` (everything, for piped input) — never by driving `bufio`/`term` directly. All three select on `ctx.Done()`, so Ctrl-C unblocks a waiting prompt instead of hanging.

**Every read that prompts the user in real time must be gated on `util.IsTerminal(cmd.InOrStdin()) && util.IsTerminal(cmd.ErrOrStderr())`** — both streams, so the user can both see the question and answer it — and must return an error naming that command's non-interactive alternative when either check fails (e.g. "use --confirm to skip the prompt", or "use --new-password, --auto-generate, or TIGER_NEW_PASSWORD"). Put the gate immediately before the prompt in the command body, not inside a shared helper, so the error can name the right flag or env var. A prompt that merely *offers* something skips silently instead of erroring, since nothing is being refused — `offerProdProtection` in `auth_login.go` is the one example. The gate is about *prompts*, not reading stdin as such — a read whose whole point is piped input (`util.ReadAll` on a here-doc or a `|`) must not be gated.

`util.IsTerminal` and `util.ReadPassword` are `var`s so tests can replace them (the `withIsTerminal` and `withReadPassword` options in `main_test.go`). `ReadPassword` needs stdin to be a real `*os.File`, so password prompts can't be driven with `cmd.SetIn`; `ReadLine`/`ReadAll` take any `io.Reader` and work fine with it.

### Interactive Terminal UIs

Interactive menus and spinners use BubbleTea v2 (`charm.land/bubbletea/v2`). Always pass all four of these options to `tea.NewProgram`:

- `tea.WithInput(cmd.InOrStdin())` — **always**, even for write-only UIs like spinners. BubbleTea enables the Kitty keyboard protocol, so Ctrl+C arrives as a key press on stdin rather than as a SIGINT; with no input plumbed, the key press is simply lost and Ctrl+C does nothing.
- `tea.WithOutput(cmd.ErrOrStderr())` — interactive UI is not the command's result (see [Streams](#streams)).
- `tea.WithContext(cmd.Context())` — so a SIGTERM caught by `main.go` unwinds the program. Skip it only when the model already has its own cancellation path that would race it, as `common.NewSpinner` does under `WaitForService`.
- `tea.WithoutSignalHandler()` — Ctrl+C is handled as a key press and other signals reach `main.go`'s handler, so BubbleTea's own handler would just be a second owner of the same shutdown.

**Every model must handle `"ctrl+c"` itself** — nothing else will. Write-only programs translate the key press into a `context.CancelFunc` call so background goroutines unwind cleanly; `spinnerModel` in `internal/common/spinner.go` is the model for that. Gate every program on both streams being terminals, just like any prompt (`common.NewSpinner` falls back to printing one message per line otherwise). Because these helpers need both streams, they take the `*cobra.Command`; code in `internal/common` can't import cobra, so `WaitForServiceArgs` and `SpinnerArgs` carry `Input`/`Output` fields the caller fills in.

## Error Handling

- Error strings start with a **lowercase** letter, so they read correctly when a caller wraps them (`fmt.Errorf("failed to get service: %w", err)`). A leading proper noun or initialism keeps its capitalization (`API key validation failed`); if that reads awkwardly, reword so the identifier isn't first (`missing required option: ClientName`). Log messages are the opposite — they start with a capital letter.
- Use `errors.New` for static error strings; use `fmt.Errorf` only when you need format verbs.
- Handle every error — never ignore one or assign it to `_` (at minimum, print a warning).
- Return errors from `RunE`; never call `os.Exit` in a command. For specific exit codes, wrap with `common.ExitWithCode`, and map API errors to exit codes with `common.ExitWithErrorFromStatusCode`. The exit codes are defined in `internal/common/errors.go`, and `main.go` honors a `common.ExitCodeError` anywhere in the error chain.
- After checking an API response's status code, nil-check the parsed body (`resp.JSON200`, etc.) before using it — a nil body indicates a bug and should return `errors.New("empty response from API")`.
- Operations that connect to a service should call `common.CheckServiceReady` first and translate `common.ErrPaused`/`common.ErrNotReady` into actionable messages.

## HTTP Requests

All outgoing HTTP requests use the shared `api.HTTPClient` defined in `internal/api/client_util.go`, which has a built-in 30-second request timeout and sets the CLI's User-Agent on every request. `NewTigerClient` and `NewTigerClientWithToken` use it automatically; for requests outside the API client, use `api.HTTPClient` directly rather than `http.DefaultClient` or a new `http.Client`. If a request needs a shorter timeout, set one via `context.WithTimeout`; if it needs a longer one, clone `api.HTTPClient` and override its `Timeout` so the User-Agent is preserved (see `upgradeHTTPClient` in `internal/cmd/upgrade.go`).

## Logging

Only the MCP server logs. `newLogger(w)` (`internal/cmd/logger_helper.go`) points the standard `log` package at `w` and returns `slog.Default()`; `tiger mcp start` calls it with `cmd.ErrOrStderr()` and hands the logger to `mcp.NewServer` and through it to the MCP SDK, so the SDK's session-lifecycle lines land on the same stream. Callers that only enumerate capabilities (`mcp list`/`get`, completion) pass nil, which becomes a discarding logger.

- There is no level configuration and no `--debug` flag: log at `Info` or above — a `Debug` call silently goes nowhere.
- Attach errors with `slog.Any("error", err)`, not a pre-formatted string. Log messages start with a capital letter (the opposite of error strings).
- Keep statements sparse: log failures that would otherwise be swallowed (the docs-proxy registration errors are the model), and startup lines that report *removed* capabilities (the docs proxy disabled, write tools skipped in read-only mode) so a client can see why a tool it expected is missing. A default `tiger mcp start` is silent — don't add per-step tracing of work that succeeded.
- Everything outside `internal/mcp` writes to stdout/stderr directly rather than logging. Never add log statements to CLI commands — print with `cmd.Print*`/`cmd.PrintErr*`, or return an error.

## MCP Server

The server exposes two kinds of tools: native Tiger tools for service management and database operations (one file per tool), and documentation tools proxied from a remote docs MCP server (`proxy.go`).

**Server state.** `NewServer(ctx, app, logger)` takes the already-loaded `*common.App` and keeps it on the `Server` along with the logger. Read-only mode, the experimental gate, and the docs-proxy settings are read once here at startup (a client must restart the server to pick up changes to those), while the analytics middleware reloads the App on every request so handlers see current config and credentials. Handlers therefore never load anything themselves — they read `s.app.GetAll()`/`GetClient()`/`GetConfig()` and log via `s.logger`.

**One file per tool**, named to match the tool (`service_create` → `service_create.go`), laid out in this order: the `<Tool>Input`/`<Tool>Output` structs and their `Schema()` methods, then `new<Tool>Tool()` returning the `*mcp.Tool`, then the `handle<Tool>` handler method on `*Server`, then helpers used only by that tool. Registration lives in `server.go` (`registerServiceTools`, `registerDatabaseTools`), so adding a tool means one new file plus one `addTool` line. Shared schema helpers and API-to-output conversion live in `utils.go`.

**Tool schemas.** Generate the base schema from the input struct with `util.Must(jsonschema.For[Input](nil))`, then enhance it in the `Schema()` method: add a description and `Examples` to every field, and use the JSON Schema properties (`Default`, `Minimum`/`Maximum`, `Enum`, `Pattern`, `MaxLength`, …) both to document values for AI assistants and to reject invalid arguments before they reach the handler. Accessing a property that doesn't exist in the generated schema panics at startup, which keeps the schema and the struct in sync. Fields without `omitempty`/`omitzero` are **required**; an optional field with a struct (non-pointer) type must use `omitzero`, because `omitempty` has no effect on struct values and the field would silently stay required.

**Read-only mode.** `cfg.ReadOnly` is a `config.ReadOnlyMode` — `all`, `prod`, or `off` (`prod` protects only services tagged `PROD`). `config.Load` normalizes every value through `parseReadOnlyMode`, which also accepts the legacy boolean spellings, so nothing downstream sees an unnormalized value. Every write/destructive CLI `RunE` and MCP handler gates through one of the two checks in `internal/common/read_only.go`:

- `common.CheckReadOnly(cfg, tag)` — for a caller that has the target's environment tag: from a fetched service via `common.ServiceEnvironmentTag(service)`, or from the tag about to be requested (`--environment` for `service create`/`fork`; the MCP versions hardcode `api.EnvironmentTagDEV`).
- `common.CheckReadOnlyByServiceID(ctx, cfg, client, projectID, serviceID)` — the same verdict from an ID, fetching the service to read its tag. The fetch happens only under `prod` (`all` refuses and `off` allows without one), and a failed fetch is a refusal.

Gotchas when adding a gated surface: a replica set is judged on its *own* tag, never its primary's; `prod` refuses *creating* a `PROD` service too (otherwise it would create services it then can't stop or delete), so `create`/`fork` gate on the tag they're about to request; the MCP server skips registering write tools only under `all` — under `prod` they stay registered and refuse per call, with a `prod` variant of the server instructions explaining that; and where the verdict is wanted as a boolean, write `CheckReadOnly(…) != nil` rather than a wrapper, so grepping one name finds every gate. `CheckReadOnly` requires a tag so it can't silently ignore `prod`; the one shortcut allowed is refusing the blanket case before a fetch the command was making anyway (`if cfg.ReadOnly.BlocksAll() { return common.ErrReadOnly }`), then still calling `CheckReadOnly` once the service arrives — `service update-password` does both.

## CLI/MCP Synchronization

- Where a CLI command and an MCP tool cover the same operation they come in pairs (`tiger service create` ↔ `service_create`). When changing one, apply the same change to the other so they stay aligned.
- **Not everything is paired, and that's fine.** Plenty of CLI commands have no MCP tool and vice versa — e.g. `tiger project use` deliberately has no tool, because a per-request MCP session must not switch a process-wide default shared with other sessions. Don't treat an unpaired surface as a gap to be filled: adding a counterpart is a design decision, not a sync fix.
- Some paired surfaces differ on purpose too (different defaults, different output shapes). Before syncing away a difference, ask whether it's intentional, and document intentional differences in code comments.
- Code that both sides need belongs in a shared package — `internal/common` for business logic with config/api dependencies, `internal/util` for small dependency-light utilities — never in `internal/cmd` or `internal/mcp`.

## Analytics

Usage tracking is automatic via middleware, so new commands and tools normally need no tracking code:

- CLI commands are tracked by `wrapCommands` with event names like `"Run tiger service create"`, capturing elapsed time, user-provided flags, and success/failure.
- MCP tool calls are tracked by `analyticsMiddleware` in `internal/mcp/server.go` with event names like `"Call service_create tool"`, along with resource reads and prompt requests.

**Sensitive data must never reach analytics.** The `ignore` list in `internal/analytics/analytics.go` filters flag and tool-parameter names (write flag names with underscores: `public-key` → `public_key`). When adding a command or tool that handles passwords, keys, SQL queries, connection strings, or similar, add the field names there. All positional arguments are tracked automatically, so accept sensitive values via flags instead — or add filtering logic in `wrapCommands` if a positional argument is unavoidable.

## Go Style

- Pre-allocate slices when the length is known: `make([]T, len(source))` with index assignment, rather than appending into an empty slice.
- Use `new(expr)` for pointer-to-value expressions — `new("mydb")`, `new(5.0)`, `new(api.DeployStatusREADY)` — instead of declaring a single-use variable to take its address (Go 1.27).
- Use multi-line struct literals when initializing more than one field, one field per line with a trailing comma; a literal setting zero or one field can stay on a single line.

## Testing

Never accept a state where tests are failing.

Command tests live in `internal/cmd`, one test file per command file, all table-driven on the shared harness in `main_test.go`:

- `runCommand(t, args, setupMock, opts...)` builds the real root command via `buildRootCmd`, injects the generated mock API client (`mocks.MockClientWithResponsesInterface`) through `app.SetClientFactory`, runs against an isolated `t.TempDir()` config directory (always passing `--analytics=false --skip-update-check`), and returns captured, ANSI-stripped stdout/stderr plus the `Execute` error. The result also carries `cfg` — the config that invocation actually resolved — so precedence can be asserted on what the command saw; it's nil when nothing loaded, which is what `checkNotLoaded` asserts for help and completion.
- Test cases use the `cmdTest` struct and run through `runCmdTests` — **one inline table per test function** (`runCmdTests(t, []cmdTest{...})`), with all shared setup above the literal; never split cases into multiple lists or append to the slice. The `wantErr`/`wantStdout`/`wantStderr` fields normally hold plain strings asserted with **exact** matching (a go-cmp diff) — that's the standard. Left unset, the output fields assert the stream is empty; when `wantErr` is set and `wantStderr` isn't, stderr is expected to be `"Error: <wantErr>\n"`. Only when output is inherently nondeterministic (random OAuth state, OS-dependent error text, huge generated schemas) may a field hold a matcher instead — `matchRegexp`, `matchPrefix`, or `matchFunc` — with a comment saying why exact matching is impossible. A case proving something *didn't* happen (a gate letting a request through, say) is still a full case with exact expected output, not a looser "wasn't refused" check. Extra assertions go in the optional `checks` slice (`checkExitCode`, `checkDefaultService`, `readConfigFile`, `readStoredCredentials`, …); helpers that build a check return `checkFunc`.
- Options configure the run: `withStdin`, `withEnv`, `withConfig` (seed config-file keys), `withStoredCredentials`, `withClientError`/`withNotLoggedIn`, `withIsTerminal`, `withReadPassword`, `withOpenBrowser`, `withContext`. Options that stub or seed process-global state are built on `withSetup` (t-scoped hooks) — a new stub of global state gets its own `with*` option there, not a standalone helper or inline stubbing in one test. Before writing a new mock/check helper, look for an existing shared one in `main_test.go` or the group's test file (`sampleService`, `expectGetService`, `httpResponse`, …).
- A case that waits on a timer — anything reaching `common.WaitForService`'s poll loop via `--wait` — sets `synctest: true` to run inside a `testing/synctest` bubble. Time there is virtual, so write the realistic durations (a one-second poll interval, a thirty-second timeout) rather than shrinking them to keep the test fast. Don't combine `synctest` with a loopback `httptest.NewServer`: real network I/O never durably blocks, so the bubble hangs until the test binary's own timeout.
- Mock expectations use `validCtx` for context arguments and **exact request structs** — don't use `gomock.Any()` as a shortcut when the expected value is known; it's acceptable only for truly nondeterministic values (e.g. a generated password), with a comment explaining why. `httpResponse(status)` and `sampleService(overrides...)` keep tables concise.
- Tests never touch the system keyring: `runCommand` starts every run with a fresh in-memory keyring (`keyring.MockInit()`), and the `TestMain`s in `internal/cmd`, `internal/common`, and `internal/config` call it too as a backstop. Those `TestMain`s also scrub inherited `TIGER_*` env vars (preserving `TIGER_*_INTEGRATION`) so tests never read the developer's environment, and pin `time.Local` to UTC so local-time output can be asserted with plain literals — never mutate `time.Local` mid-run.
- Seed a config file with `withConfig`, or `writeConfigFile(t, dir, values)` outside `runCommand` — it writes only the given keys, so everything else resolves from defaults. Tests elsewhere that just need a `*config.Config` build one as a struct literal.
- Order test cases by the command's execution flow: auth errors, argument/flag validation, the read-only gate, network/API errors, nil response body, success paths (table, json, yaml), then remaining flags and edge cases — so the table reads like a walkthrough of the function.
- Slice tests by command, never by feature. A behavior that spans commands (the read-only gate, say) is tested as cases in each affected command's table, not as one cross-command test looping over commands — each command's table must stay the single place where its full behavior is read and extended.
- Test commands as a whole through `runCommand`; don't unit-test individual helpers unless their behavior is genuinely unreachable through a command (a Bubble Tea model that needs a real TTY, SQL builders that only execute over a live connection). Such a test keeps a comment saying why it's helper-level, and the command tests stub the helper through a seam — a package-level `var` replaced by a `withSetup`-based option (`withSelectProject` in `auth_login_test.go` is the model). Commands that reach a real database (pgx) or spawn external binaries test their error paths only — the success paths are covered by `integration_test.go`.

## Documentation

After changing commands, MCP tools, config options, or flags, check and update **README.md** (user-facing documentation), **CLAUDE.md** (this file), and **docs/development.md** (development guide) to keep them in sync with the implementation.

Keep this file concise and high-level — it's pulled into every agent session, so unnecessary detail bloats context and distracts more than it helps. Describe the intent and the pattern; let the code be the source of truth for the details.

## CI

`test.yml` runs the test suite on pull requests and pushes to `main`. CI needs no keyring setup — `TestMain` swaps in the in-memory mock.

## Releases

Creating a release automatically triggers a build and publish via GoReleaser. To create a new release, follow these steps:

1. Make sure you are on `main` and up to date (`git pull`).
2. Find the latest tag to determine the current version: `git tag --sort=-v:refname | head -5`
3. Ask the user whether this is a **major**, **minor**, or **patch** bump — do not assume — and compute the next version.
4. Create the release, which creates the tag and triggers the workflow:
   ```bash
   gh release create v<NEXT_VERSION> --generate-notes --latest
   ```
5. If the user asks to watch the workflow:
   ```bash
   gh run watch $(gh run list -w Release -b v<NEXT_VERSION> -L 1 --json databaseId -q '.[0].databaseId')
   ```

The release's tag triggers `release.yml`, which runs GoReleaser (configured in `.goreleaser.yaml`) to build and publish everywhere at once: GitHub Releases binaries for macOS/Linux/Windows, the Homebrew formula in `timescale/homebrew-tap`, binaries in the `tiger-cli-releases` S3 bucket behind `https://cli.tigerdata.com` (used by the install script and Homebrew), Debian/RPM packages on PackageCloud, the Docker image on `ghcr.io/timescale/tiger-cli`, and the `server.json` manifest (rendered from `server-template.json`) to the MCP registry. Always release this way — never push a tag directly, so every release has a GitHub release behind it.
