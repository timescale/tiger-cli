package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceUpdatePasswordCmd creates a new update-password command
func buildServiceUpdatePasswordCmd(app *common.App) *cobra.Command {
	var updatePasswordValue string
	var autoGenerate bool

	cmd := &cobra.Command{
		Use:   "update-password [service-id]",
		Short: "Update the master password for a service",
		Long: `Update the master password for a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command updates the master password for the
'tsdbadmin' user used to authenticate to the database service.

A read replica ID is rejected — read replicas share the primary's credentials,
so update the password on the primary instead.`,
		Example: `  # Update password for default service, interactively prompts
  tiger service update-password

  # Update password for default service
  tiger service update-password --new-password new-secure-password

  # Update password for specific service
  tiger service update-password svc-12345 --new-password new-secure-password

  # Update password using environment variable (TIGER_NEW_PASSWORD)
  export TIGER_NEW_PASSWORD="new-secure-password"
  tiger service update-password svc-12345

  # Update password and save to .pgpass (default behavior)
  tiger service update-password svc-12345 --new-password new-secure-password

  # Update password without saving (using global flag)
  tiger service update-password svc-12345 --new-password new-secure-password --password-storage none

  # Auto-generate a secure password
  tiger service update-password --auto-generate`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Refuse without an API call under read_only=all. prod needs the tag,
			// so the real gate waits for the fetch below.
			if cfg.ReadOnly.BlocksAll() {
				return common.ErrReadOnly
			}

			// Determine service ID
			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			// The password comes from the flag, falling back to the env var
			password := updatePasswordValue
			if password == "" {
				password = os.Getenv("TIGER_NEW_PASSWORD")
			}
			if autoGenerate && password != "" {
				return fmt.Errorf("cannot use --auto-generate and --new-password together")
			}

			ctx := cmd.Context()

			// Fetch service details
			serviceResp, err := client.GetServiceWithResponse(ctx, projectID, serviceID)
			if err != nil {
				return fmt.Errorf("failed to get service details: %w", err)
			}
			if serviceResp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(serviceResp.StatusCode(), serviceResp.JSON4XX)
			}

			if serviceResp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			service := *serviceResp.JSON200

			// The prod half of the gate, riding on the fetch above and still
			// ahead of any prompt or write.
			if err := common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(service)); err != nil {
				return err
			}

			// A read replica has no separate password to rotate.
			if common.IsReadReplica(service) {
				return fmt.Errorf("%q is a read replica; update the password on its primary service %q instead",
					serviceID, util.DerefStr(service.ForkedFrom.ServiceID))
			}

			if autoGenerate {
				// Auto-generate password using existing function
				if _, err := resetServicePassword(ctx, cmd, cfg, client, service, "tsdbadmin", ""); err != nil {
					return err
				}
			} else if password == "" {
				// Interactive prompt - check if we're in a terminal
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return fmt.Errorf("TTY not detected - use --new-password flag, --auto-generate flag, or TIGER_NEW_PASSWORD environment variable")
				}
				_, err := promptAndResetPassword(ctx, cmd, cfg, client, service, "tsdbadmin")
				if err != nil {
					return err
				}
			} else {
				if _, err := resetServicePassword(ctx, cmd, cfg, client, service, "tsdbadmin", password); err != nil {
					return err
				}
			}

			cmd.PrintErrf("✅ Master password for 'tsdbadmin' user updated successfully\n")
			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&updatePasswordValue, "new-password", "", "New password for the tsdbadmin user (can also be set via TIGER_NEW_PASSWORD env var)")
	cmd.Flags().BoolVar(&autoGenerate, "auto-generate", false, "Auto-generate a secure password")
	cmd.MarkFlagsMutuallyExclusive("new-password", "auto-generate")
	return cmd
}
