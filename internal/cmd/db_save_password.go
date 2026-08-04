package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbSavePasswordCmd() *cobra.Command {
	var dbSavePasswordRole string
	var dbSavePasswordValue string

	cmd := &cobra.Command{
		Use:   "save-password [service-id]",
		Short: "Save password for a database service",
		Long: `Save a password for a database service to configured password storage.

The service ID can be provided as an argument or will use the default service
from your configuration. The password can be provided via:
1. --password flag with explicit value (highest precedence)
2. TIGER_NEW_PASSWORD environment variable
3. Interactive prompt (if neither provided)

The password will be saved according to your --password-storage setting
(keyring, pgpass, or none).

Examples:
  # Save password with explicit value (highest precedence)
  tiger db save-password svc-12345 --password=your-password

  # Using environment variable
  export TIGER_NEW_PASSWORD=your-password
  tiger db save-password svc-12345

  # Interactive password prompt (when neither flag nor env var provided)
  tiger db save-password svc-12345

  # Save password for custom role
  tiger db save-password svc-12345 --password=your-password --role readonly

  # Save to specific storage location
  tiger db save-password svc-12345 --password=your-password --password-storage pgpass`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := common.LoadConfig(cmd.Context(), cmd.Flags())
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			// Resolve the target so a read replica id stores the password against
			// its parent primary: replicas share the primary's credentials, and
			// connect/test-connection look the password up against the primary.
			target, err := lookupConnectionTarget(cmd, cfg, args)
			if err != nil {
				return err
			}
			service := target.CredentialService

			// Determine password based on precedence:
			// 1. --password flag with value
			// 2. TIGER_NEW_PASSWORD environment variable
			// 3. Interactive prompt
			var passwordToSave string

			if cmd.Flags().Changed("password") {
				// --password flag was provided
				passwordToSave = dbSavePasswordValue
				if passwordToSave == "" {
					return fmt.Errorf("password cannot be empty when provided via --password flag")
				}
			} else if envPassword := os.Getenv("TIGER_NEW_PASSWORD"); envPassword != "" {
				// Use environment variable
				passwordToSave = envPassword
			} else {
				// Interactive prompt - check if we're in a terminal
				if !checkStdinIsTTY() {
					return fmt.Errorf("TTY not detected - password required. Use --password flag or TIGER_NEW_PASSWORD environment variable")
				}

				fmt.Fprint(cmd.OutOrStdout(), "Enter password: ")
				passwordToSave, err = readString(cmd.Context(), readPasswordFromTerminal)
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout()) // Print newline after hidden input
				if passwordToSave == "" {
					return fmt.Errorf("password cannot be empty")
				}
			}

			// Save password using configured storage
			storage := common.GetPasswordStorage(cfg.Config)
			if err := storage.Save(service, passwordToSave, dbSavePasswordRole); err != nil {
				return fmt.Errorf("failed to save password: %w", err)
			}

			if target.IsReplica {
				fmt.Fprintf(cmd.ErrOrStderr(), "Read replicas share the primary's credentials; saving against primary %s.\n",
					*service.ServiceId)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Password saved successfully for service %s (role: %s)\n",
				*service.ServiceId, dbSavePasswordRole)
			return nil
		},
	}

	// Add flags for db save-password command
	cmd.Flags().StringVarP(&dbSavePasswordValue, "password", "p", "", "Password to save")
	cmd.Flags().StringVar(&dbSavePasswordRole, "role", "tsdbadmin", "Database role/username")

	return cmd
}
