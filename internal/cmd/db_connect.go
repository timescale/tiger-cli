package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbConnectCmd() *cobra.Command {
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
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := common.LoadConfig(cmd.Context())
			if err != nil {
				return err
			}

			// Separate service ID from additional psql flags
			serviceArgs, psqlFlags := separateServiceAndPsqlArgs(cmd, args)

			target, err := lookupConnectionTarget(cmd, cfg, serviceArgs)
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
			details, err := selectConnection(cmd.Context(), cmd, cfg.Client, cfg.ProjectID, target, opts, dbConnectNoReplicaPrompt)
			if err != nil {
				return err
			}
			if details == nil {
				return nil
			}

			// Read replicas share the primary's credentials, so password storage
			// and recovery always operate on the credential service.
			return connectWithPasswordMenu(cmd.Context(), cmd, cfg.Client, target.CredentialService, details, psqlPath, psqlFlags)
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

// launchPsql launches psql using the connection string and additional flags.
// It retrieves the password from storage and sets PGPASSWORD environment variable.
func launchPsql(details *common.ConnectionDetails, psqlPath string, additionalFlags []string, service api.Service, cmd *cobra.Command) error {
	psqlCmd := buildPsqlCommand(details, psqlPath, additionalFlags, service, cmd)
	return psqlCmd.Run()
}

// buildPsqlCommand creates the psql command with proper environment setup
func buildPsqlCommand(details *common.ConnectionDetails, psqlPath string, additionalFlags []string, service api.Service, cmd *cobra.Command) *exec.Cmd {
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
		storage := common.GetPasswordStorage()
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
