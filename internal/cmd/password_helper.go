package cmd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// updateAndSaveServicePassword updates a service password via API and saves it locally.
// It handles the API call and password storage. credentials are the service's
// own: the service password endpoint takes only services.
func updateAndSaveServicePassword(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	client api.ClientWithResponsesInterface,
	credentials common.PasswordTarget,
	newPassword string,
	role string,
) error {
	// Call API to update password
	updateReq := api.UpdatePasswordInput{Password: newPassword}
	resp, err := client.UpdatePasswordWithResponse(ctx, credentials.ProjectID, credentials.ID, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if resp.StatusCode() != http.StatusNoContent {
		return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	// Save password locally
	if result, err := common.SavePasswordWithResult(cfg, credentials, newPassword, role); err != nil {
		cmd.PrintErrf("Warning: could not save password: %v\n", err)
	} else if result.Success {
		cmd.PrintErrf("%s\n", result.Message)
		cmd.PrintErrf("View it with: tiger service get %s --with-password\n", credentials.ID)
	}

	return nil
}

// resetServicePassword resets the password via API. If newPassword is empty, generates one.
func resetServicePassword(ctx context.Context, cmd *cobra.Command, cfg *config.Config, client api.ClientWithResponsesInterface, credentials common.PasswordTarget, role string, newPassword string) (string, error) {
	// Generate password if not provided
	if newPassword == "" {
		var err error
		if newPassword, err = common.GenerateSecurePassword(32); err != nil {
			return "", fmt.Errorf("failed to generate new password: %w", err)
		}
	}

	// Update and save password
	if err := updateAndSaveServicePassword(ctx, cmd, cfg, client, credentials, newPassword, role); err != nil {
		return "", err
	}
	return newPassword, nil
}

// promptAndResetPassword prompts for a new password and resets it via API.
// If the user leaves the password empty, a secure password is generated.
// Returns the new password on success.
func promptAndResetPassword(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	client api.ClientWithResponsesInterface,
	credentials common.PasswordTarget,
	role string,
) (string, error) {
	cmd.PrintErr("Enter new password (leave empty to generate): ")
	newPassword, err := util.ReadPassword(ctx, cmd.InOrStdin())
	cmd.PrintErrln() // newline after password entry
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	return resetServicePassword(ctx, cmd, cfg, client, credentials, role, newPassword)
}

// printPasswordStorageResult reports the outcome of saving a password and
// returns whether it was saved.
func printPasswordStorageResult(cmd *cobra.Command, result common.PasswordStorageResult) bool {
	if result.Method == "none" && result.Message == "No password provided" {
		// Don't output anything for empty password
		return false
	}

	// Only a real storage failure is a warning; the "none" method is informational.
	if result.Success {
		cmd.PrintErrf("%s\n", result.Message)
		return true
	} else if result.Method == "none" {
		cmd.PrintErrf("%s\n", result.Message)
	} else {
		cmd.PrintErrf("Warning: %s\n", result.Message)
	}
	return false
}

// handlePasswordSaving handles saving password using the configured storage
// method and displaying appropriate messages. Returns true if the password was
// successfully saved, or false if not.
func handlePasswordSaving(cmd *cobra.Command, cfg *config.Config, target common.PasswordTarget, initialPassword string) bool {
	// Note: We don't fail the service creation if password saving fails
	// The error is handled by displaying the appropriate message below
	result, _ := common.SavePasswordWithResult(cfg, target, initialPassword, "tsdbadmin")
	return printPasswordStorageResult(cmd, result)
}

// printConnectMessage tells the user how to connect to a new service or branch.
func printConnectMessage(cmd *cobra.Command, passwordSaved, noSetDefault bool, serviceID string) {
	if !passwordSaved {
		// We can't connect if no password was saved, so don't show message
		return
	} else if noSetDefault {
		// If the service wasn't set as the default, include the serviceID in the command
		cmd.PrintErrf("Connect with: tiger db psql %s\n", serviceID)
	} else {
		// If the service was set as the default, no need to include the serviceID in the command
		cmd.PrintErrf("Connect with: tiger db psql\n")
	}
}
