package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// updateAndSavePassword sets the password of credentials' owner, a service or
// a branch, through the matching API endpoint, and saves it locally.
func updateAndSavePassword(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	client api.ClientWithResponsesInterface,
	credentials common.CredentialOwner,
	newPassword string,
	role string,
) error {
	if err := updatePassword(ctx, client, credentials, newPassword); err != nil {
		return err
	}

	// Save password locally
	if result, err := common.SavePasswordWithResult(cfg, credentials.PasswordTarget, newPassword, role); err != nil {
		cmd.PrintErrf("Warning: could not save password: %v\n", err)
	} else if result.Success {
		cmd.PrintErrf("%s\n", result.Message)
		// Only service get can show a stored password.
		if credentials.BranchServiceID == "" {
			cmd.PrintErrf("View it with: tiger service get %s --with-password\n", credentials.ID)
		}
	}

	return nil
}

// updatePassword sets the password of credentials' owner through the service
// or branch password endpoint.
func updatePassword(ctx context.Context, client api.ClientWithResponsesInterface, credentials common.CredentialOwner, newPassword string) error {
	input := api.UpdatePasswordInput{Password: newPassword}
	if credentials.BranchServiceID != "" {
		return common.Status204(client.UpdateBranchPasswordWithResponse(ctx, credentials.ProjectID, credentials.BranchServiceID, credentials.ID, input))
	}
	return common.Status204(client.UpdatePasswordWithResponse(ctx, credentials.ProjectID, credentials.ID, input))
}

// resetPassword resets the password of credentials' owner via the API. If
// newPassword is empty, it generates one.
func resetPassword(ctx context.Context, cmd *cobra.Command, cfg *config.Config, client api.ClientWithResponsesInterface, credentials common.CredentialOwner, role string, newPassword string) (string, error) {
	// Generate password if not provided
	if newPassword == "" {
		var err error
		if newPassword, err = common.GenerateSecurePassword(32); err != nil {
			return "", fmt.Errorf("failed to generate new password: %w", err)
		}
	}

	if err := updateAndSavePassword(ctx, cmd, cfg, client, credentials, newPassword, role); err != nil {
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
	credentials common.CredentialOwner,
	role string,
) (string, error) {
	cmd.PrintErr("Enter new password (leave empty to generate): ")
	newPassword, err := util.ReadPassword(ctx, cmd.InOrStdin())
	cmd.PrintErrln() // newline after password entry
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	return resetPassword(ctx, cmd, cfg, client, credentials, role, newPassword)
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
