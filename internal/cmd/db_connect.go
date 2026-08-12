package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildDbConnectCmd(app *common.App) *cobra.Command {
	var dbConnectPooled bool
	var dbConnectRole string
	var dbConnectReadOnly bool
	var dbConnectNoReplicaPrompt bool

	cmd := &cobra.Command{
		Use:     "connect [service-id]",
		Aliases: []string{"psql"},
		Short:   "Connect to a database",
		Long: `Connect to a database service using psql client.

The service ID can be provided as an argument or will use the default service
from your configuration. This command will launch an interactive psql session
with the appropriate connection parameters.

Authentication is handled automatically using:
1. Stored password (keyring, ~/.pgpass, or none based on --password-storage setting)
2. PGPASSWORD environment variable
3. If authentication fails, offers interactive options:
   - Enter password manually (will be saved for future use)
   - Reset password (update or generates a new password via the API)

Use --read-only to open the psql session in Tiger Cloud's immutable read-only
mode (writes and DDL are rejected by the server). The global read_only config
option (or TIGER_READ_ONLY=true) also forces this behavior, so sessions started
while read-only mode is on are always read-only.

When run in an interactive terminal, this command checks whether the service has
any read replicas. If it does, it offers to connect to one of them instead of the
primary. Use --no-replica-prompt to skip this prompt and always connect to the
requested service. The prompt is automatically skipped when stdin is not a
terminal (e.g. in scripts) or when the service has no read replicas.

You can also pass a read replica set ID to connect straight to that replica,
skipping the prompt. Read replicas share the primary's credentials.

Examples:
  # Connect to default service
  tiger db connect
  tiger db psql

  # Connect directly to a read replica by its ID
  tiger db connect rep1234567

  # Connect without the read replica prompt
  tiger db connect svc-12345 --no-replica-prompt

  # Connect to specific service
  tiger db connect svc-12345
  tiger db psql svc-12345

  # Connect using connection pooler
  tiger db connect svc-12345 --pooled
  tiger db psql svc-12345 --pooled

  # Connect with custom role/username
  tiger db connect svc-12345 --role readonly
  tiger db psql svc-12345 --role readonly

  # Connect in read-only mode (writes and DDL are rejected by the server)
  tiger db connect svc-12345 --read-only

  # Pass additional flags to psql (use -- to separate)
  tiger db connect svc-12345 -- --single-transaction --quiet
  tiger db psql svc-12345 -- -c "SELECT version();" --no-psqlrc`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: serviceIDCompletion(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, _, _, err := app.GetAll()
			if err != nil {
				return err
			}

			// Separate service ID from additional psql flags
			serviceArgs, psqlFlags := separateServiceAndPsqlArgs(cmd, args)

			target, err := lookupConnectionTarget(cmd, app, serviceArgs)
			if err != nil {
				return err
			}

			// Check if psql is available
			psqlPath, err := exec.LookPath("psql")
			if err != nil {
				return fmt.Errorf("psql client not found. Please install PostgreSQL client tools")
			}

			opts := common.ConnectionDetailsOptions{
				Pooled:   dbConnectPooled,
				Role:     dbConnectRole,
				ReadOnly: dbConnectReadOnly || cfg.ReadOnly,
			}

			// Connects straight to a replica named by ID, or offers the interactive
			// replica menu for a primary. Returns nil details if the user cancels.
			details, err := selectConnection(cmd.Context(), cmd, app, target, opts, dbConnectNoReplicaPrompt)
			if err != nil {
				return err
			}
			if details == nil {
				return nil
			}

			// Read replicas share the primary's credentials, so password storage
			// and recovery always operate on the credential service.
			return connectWithPasswordMenu(cmd.Context(), cmd, app, target.CredentialService, details, psqlPath, psqlFlags)
		},
	}

	// Add flags for db connect command (works for both connect and psql)
	cmd.Flags().BoolVar(&dbConnectPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectRole, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().BoolVar(&dbConnectReadOnly, "read-only", false, "Open the connection in Tiger Cloud's immutable read-only mode")
	cmd.Flags().BoolVar(&dbConnectNoReplicaPrompt, "no-replica-prompt", false, "Don't prompt to connect to a read replica")

	return cmd
}

// ArgsLenAtDashProvider defines the interface for getting ArgsLenAtDash
type ArgsLenAtDashProvider interface {
	ArgsLenAtDash() int
}

// separateServiceAndPsqlArgs separates service arguments from psql flags using Cobra's ArgsLenAtDash
func separateServiceAndPsqlArgs(cmd ArgsLenAtDashProvider, args []string) ([]string, []string) {
	var serviceArgs []string
	psqlFlags := []string{}

	argsLenAtDash := cmd.ArgsLenAtDash()
	if argsLenAtDash >= 0 {
		// There was a -- separator
		serviceArgs = args[:argsLenAtDash]
		psqlFlags = args[argsLenAtDash:]
	} else {
		// No -- separator
		serviceArgs = args
	}

	return serviceArgs, psqlFlags
}

// selectConnection returns the connection details for `tiger db connect`. A
// replica target connects straight through; a primary in an interactive
// terminal is offered a menu to pick the primary or one of its replicas (nil
// details means the user cancelled).
func selectConnection(
	ctx context.Context,
	cmd *cobra.Command,
	app *common.App,
	target *common.ConnectionTarget,
	opts common.ConnectionDetailsOptions,
	noReplicaPrompt bool,
) (*common.ConnectionDetails, error) {
	cfg, client, projectID, err := app.GetAll()
	if err != nil {
		return nil, err
	}

	// chosen is what we connect to; the menu below may replace it with a replica.
	chosen := target

	// Offer the replica menu only for a primary on an interactive terminal.
	if !target.IsReplica && !noReplicaPrompt && util.IsTerminal(cmd.InOrStdin()) && util.IsTerminal(cmd.ErrOrStderr()) {
		primary := target.ConnectionService
		replicas, err := fetchReplicaSets(ctx, client, projectID, util.DerefStr(primary.ServiceId))
		if err != nil {
			// Don't block the connection if we can't list replicas.
			cmd.PrintErrf("Warning: could not list read replicas: %v\n", err)
		} else if connectable := connectableReplicas(replicas); len(connectable) > 0 {
			choice, err := selectConnectTargetOption(cmd, primary, connectable)
			if err != nil {
				return nil, err
			}
			switch choice.kind {
			case targetCancel:
				return nil, nil
			case targetReplica:
				chosen = common.NewReplicaConnectionTarget(primary, *choice.replica)
			}
		}
	}

	details, err := buildConnectionDetailsForTarget(cmd, cfg, chosen, opts)
	if err != nil {
		return nil, err
	}

	if chosen.IsReplica {
		cmd.PrintErrf("Connecting to read replica '%s'...\n", util.DerefStr(chosen.ConnectionService.Name))
	}
	return details, nil
}

// connectableReplicas filters to active read replicas that expose an endpoint.
func connectableReplicas(replicas []api.ReadReplicaSet) []api.ReadReplicaSet {
	var out []api.ReadReplicaSet
	for _, r := range replicas {
		if r.Status != nil && *r.Status == api.ReadReplicaSetStatusActive && r.Endpoint != nil {
			out = append(out, r)
		}
	}
	return out
}

// fetchReplicaSets retrieves the read replica sets for a service.
func fetchReplicaSets(ctx context.Context, client api.ClientWithResponsesInterface, projectID, serviceID string) ([]api.ReadReplicaSet, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.GetReplicaSetsWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, nil
	}

	return *resp.JSON200, nil
}

// connectTargetKind enumerates the choices in the connect target menu.
type connectTargetKind int

const (
	targetPrimary connectTargetKind = iota
	targetReplica
	targetCancel
)

// connectTargetChoice is one menu entry: its display label and the action it represents.
type connectTargetChoice struct {
	kind    connectTargetKind
	replica *api.ReadReplicaSet
	label   string
}

// connectTargetModel is the Bubble Tea model for selecting a connection target.
type connectTargetModel struct {
	choices []connectTargetChoice
	cursor  int
	chosen  connectTargetChoice
}

func newConnectTargetModel(primary api.Service, replicas []api.ReadReplicaSet) connectTargetModel {
	choices := []connectTargetChoice{{
		kind:  targetPrimary,
		label: fmt.Sprintf("Connect to primary service (%s)", util.DerefStr(primary.ServiceId)),
	}}

	for i := range replicas {
		choices = append(choices, connectTargetChoice{
			kind:    targetReplica,
			replica: &replicas[i],
			label:   fmt.Sprintf("Connect to read replica '%s'", util.DerefStr(replicas[i].Name)),
		})
	}

	choices = append(choices, connectTargetChoice{kind: targetCancel, label: "Cancel"})

	return connectTargetModel{
		choices: choices,
		// Default to cancel so quitting (ctrl+c/q) is a no-op connection.
		chosen: connectTargetChoice{kind: targetCancel},
	}
}

func (m connectTargetModel) Init() tea.Cmd {
	return nil
}

func (m connectTargetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch key := msg.String(); key {
		case "ctrl+c", "q":
			m.chosen = connectTargetChoice{kind: targetCancel}
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.chosen = m.choices[m.cursor]
			return m, tea.Quit
		default:
			// Number keys jump straight to that option ('1' -> first, etc.).
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				if idx := int(key[0] - '1'); idx < len(m.choices) {
					m.cursor = idx
					m.chosen = m.choices[idx]
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m connectTargetModel) View() tea.View {
	s := "How would you like to connect?\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, choice.label)
	}

	s += "\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to cancel"
	return tea.NewView(s)
}

// selectConnectTargetOption shows the interactive menu for choosing a
// connection target.
func selectConnectTargetOption(cmd *cobra.Command, primary api.Service, replicas []api.ReadReplicaSet) (connectTargetChoice, error) {
	model := newConnectTargetModel(primary, replicas)

	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
	finalModel, err := program.Run()
	if err != nil {
		return connectTargetChoice{kind: targetCancel}, fmt.Errorf("failed to run connect menu: %w", err)
	}

	return finalModel.(connectTargetModel).chosen, nil
}

// connectWithPasswordMenu handles the connection flow if the stored password is invalid
// Offers an interactive menu to enter the password manually or reset it
func connectWithPasswordMenu(
	ctx context.Context,
	cmd *cobra.Command,
	app *common.App,
	service api.Service,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
) error {
	cfg, client, _, err := app.GetAll()
	if err != nil {
		return err
	}

	// Interactive mode: Get stored password (if any)
	storage := common.GetPasswordStorage(cfg)
	storedPassword, err := storage.Get(service, details.Role)
	if err != nil {
		cmd.PrintErrf("Warning: could not retrieve stored password: %v\n", err)
	}

	// Try to connect with stored password first
	err = testConnectionWithPassword(ctx, details, storedPassword)
	if err == nil {
		// Password works, launch psql
		return launchPsql(cfg, details, psqlPath, psqlFlags, service, cmd)
	}

	// Check if it's an auth error
	if !isAuthenticationError(err) {
		// Non-auth error (network, timeout, etc.) - report it directly
		return err
	}
	// Auth failed with stored password, continue to recovery menu
	cmd.PrintErrf("%s\nStored password is likely invalid or expired.\n\n", err.Error())

	// Check if we're in a TTY for interactive menu
	if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
		return fmt.Errorf("authentication failed and no TTY available for interactive password entry")
	}

	// Interactive recovery loop
	// Only allow password reset for admin role
	canResetPassword := details.Role == "tsdbadmin"
	for {
		option, err := selectPasswordRecoveryOption(cmd, canResetPassword)
		if err != nil {
			return err
		}

		switch option {
		case optionEnterPassword:
			// Prompt for password
			cmd.PrintErr("Enter password: ")
			password, err := util.ReadPassword(ctx, cmd.InOrStdin())
			cmd.PrintErrln() // newline after password entry
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil // user cancelled
				}
				cmd.PrintErrf("Error reading password: %v\n\n", err)
				continue
			}

			// Test, save, and launch
			details.Password = password
			if err = testSaveAndLaunchPsqlWithPassword(ctx, cmd, cfg, details, psqlPath, psqlFlags, service); err != nil {
				if isAuthenticationError(err) {
					cmd.PrintErrf("Password incorrect. Please try again.\n\n")
					continue
				}
				return fmt.Errorf("connection failed: %w", err)
			}
			return nil

		case optionResetPassword:
			// Prompt and reset
			password, err := promptAndResetPassword(ctx, cmd, cfg, client, service, details.Role)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil // user cancelled
				}
				cmd.PrintErrf("Error resetting password: %v\n\n", err)
				continue
			}
			cmd.PrintErrf("✅ Master password for '%s' user updated successfully\n", details.Role)
			// Launch psql (password is now in storage)
			details.Password = password
			return launchPsql(cfg, details, psqlPath, psqlFlags, service, cmd)

		case optionExit:
			return nil
		}
	}
}

// testConnectionWithPassword tests database connectivity with a specific password
// Returns nil on success, error on failure
func testConnectionWithPassword(ctx context.Context, details *common.ConnectionDetails, password string) error {
	// copy details with provided password
	copyDetails := *details
	copyDetails.Password = password
	connStr := copyDetails.String()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	return conn.Close(ctx)
}

// isAuthenticationError checks if the error is a PostgreSQL authentication failure
func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	// Check for PostgreSQL error code 28P01 (invalid_password) or 28000 (invalid_authorization_specification)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "28P01" || pgErr.Code == "28000"
	}
	return false
}

// passwordRecoveryOption represents the user's choice in the password recovery menu
type passwordRecoveryOption int

const (
	optionEnterPassword passwordRecoveryOption = iota
	optionResetPassword
	optionExit
)

// passwordRecoveryModel is the Bubble Tea model for password recovery selection
type passwordRecoveryModel struct {
	options          []string
	optionMap        []passwordRecoveryOption // maps cursor position to option enum
	cursor           int
	selected         passwordRecoveryOption
	canResetPassword bool
}

func newPasswordRecoveryModel(canResetPassword bool) passwordRecoveryModel {
	options := []string{"Enter password manually"}
	optionMap := []passwordRecoveryOption{optionEnterPassword}

	if canResetPassword {
		options = append(options, "Update/reset password")
		optionMap = append(optionMap, optionResetPassword)
	}

	options = append(options, "Exit")
	optionMap = append(optionMap, optionExit)

	return passwordRecoveryModel{
		options:          options,
		optionMap:        optionMap,
		cursor:           0,
		canResetPassword: canResetPassword,
	}
}

func (m passwordRecoveryModel) Init() tea.Cmd {
	return nil
}

func (m passwordRecoveryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.selected = optionExit
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.selected = m.optionMap[m.cursor]
			return m, tea.Quit
		default:
			// Handle number keys based on available options
			if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
				idx := int(msg.String()[0] - '1') // '1' -> 0, '2' -> 1, etc.
				if idx >= 0 && idx < len(m.options) {
					m.cursor = idx
					m.selected = m.optionMap[idx]
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m passwordRecoveryModel) View() tea.View {
	s := "What would you like to do?\n\n"

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, option)
	}

	s += "\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to quit"
	return tea.NewView(s)
}

// selectPasswordRecoveryOption shows the interactive menu for password recovery
// canResetPassword controls whether the "Update/reset password" option is shown
func selectPasswordRecoveryOption(cmd *cobra.Command, canResetPassword bool) (passwordRecoveryOption, error) {
	model := newPasswordRecoveryModel(canResetPassword)

	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
	finalModel, err := program.Run()
	if err != nil {
		return optionExit, fmt.Errorf("failed to run password recovery menu: %w", err)
	}

	result := finalModel.(passwordRecoveryModel)
	return result.selected, nil
}

// testSaveAndLaunchPsqlWithPassword tests a password, saves it if valid, and launches psql.
// Returns a retryable error if authentication fails, or a fatal error otherwise.
func testSaveAndLaunchPsqlWithPassword(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
	service api.Service,
) error {
	// Test the password
	if err := testConnectionWithPassword(ctx, details, details.Password); err != nil {
		return err
	}

	// Password works! Save it
	result, saveErr := common.SavePasswordWithResult(cfg, service, details.Password, details.Role)
	if saveErr != nil {
		cmd.PrintErrf("Warning: could not save password: %v\n", saveErr)
	} else if result.Success {
		cmd.PrintErrf("%s\n", result.Message)
	}

	// Launch psql
	return launchPsql(cfg, details, psqlPath, psqlFlags, service, cmd)
}

// launchPsql launches psql using the connection string and additional flags.
// It retrieves the password from storage and sets PGPASSWORD environment variable.
func launchPsql(cfg *config.Config, details *common.ConnectionDetails, psqlPath string, additionalFlags []string, service api.Service, cmd *cobra.Command) error {
	psqlCmd := buildPsqlCommand(cfg, details, psqlPath, additionalFlags, service, cmd)
	return psqlCmd.Run()
}

// buildPsqlCommand creates the psql command with proper environment setup
func buildPsqlCommand(cfg *config.Config, details *common.ConnectionDetails, psqlPath string, additionalFlags []string, service api.Service, cmd *cobra.Command) *exec.Cmd {
	password := details.Password
	// Ensure we don't include password in the connection string to make it not show up in process lists
	// Passwords are passed via PGPASSWORD environment variable (see below)
	detailsCopy := *details
	detailsCopy.Password = ""
	connectionString := detailsCopy.String()
	// Build command arguments: connection string first, then additional flags
	args := []string{connectionString}
	args = append(args, additionalFlags...)

	psqlCmd := exec.Command(psqlPath, args...)

	// Use cmd's input/output streams for testability while maintaining CLI behavior
	psqlCmd.Stdin = cmd.InOrStdin()
	psqlCmd.Stdout = cmd.OutOrStdout()
	psqlCmd.Stderr = cmd.ErrOrStderr()

	// Use provided password directly if available
	if password != "" {
		psqlCmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	} else {
		storage := common.GetPasswordStorage(cfg)
		// Only set PGPASSWORD for keyring storage method
		// pgpass storage relies on psql automatically reading ~/.pgpass file
		if _, isKeyring := storage.(*common.KeyringStorage); isKeyring {
			if storedPassword, err := storage.Get(service, details.Role); err == nil && storedPassword != "" {
				// Set PGPASSWORD environment variable for psql when using keyring
				psqlCmd.Env = append(os.Environ(), "PGPASSWORD="+storedPassword)
			}
			// Note: If keyring password retrieval fails, we let psql try without it
			// This allows fallback to other authentication methods
		}
	}

	return psqlCmd
}
