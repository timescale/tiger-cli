package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildDbSavePasswordCmd(app *common.App) *cobra.Command {
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
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, _, err := app.GetAll()
			if err != nil {
				return err
			}

			// Resolve the target so a read replica id stores the password against
			// its parent primary: replicas share the primary's credentials, and
			// connect/test-connection look the password up against the primary.
			target, err := lookupConnectionTarget(cmd, app, args)
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
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return fmt.Errorf("TTY not detected - password required. Use --password flag or TIGER_NEW_PASSWORD environment variable")
				}

				cmd.PrintErr("Enter password: ")
				passwordToSave, err = util.ReadPassword(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}
				cmd.PrintErrln() // Print newline after hidden input
				if passwordToSave == "" {
					return fmt.Errorf("password cannot be empty")
				}
			}

			// Save password using configured storage
			storage := common.GetPasswordStorage(cfg)
			if err := storage.Save(service, passwordToSave, dbSavePasswordRole); err != nil {
				return fmt.Errorf("failed to save password: %w", err)
			}

			if target.IsReplica {
				cmd.PrintErrf("Read replicas share the primary's credentials; saving against primary %s.\n",
					*service.ServiceID)
			}
			cmd.PrintErrf("Password saved successfully for service %s (role: %s)\n",
				*service.ServiceID, dbSavePasswordRole)
			return nil
		},
	}

	// Add flags for db save-password command
	cmd.Flags().StringVarP(&dbSavePasswordValue, "password", "p", "", "Password to save")
	cmd.Flags().StringVar(&dbSavePasswordRole, "role", "tsdbadmin", "Database role/username")

	return cmd
}
