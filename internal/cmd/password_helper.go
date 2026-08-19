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

// updateAndSaveServicePassword updates a service password via API and saves it locally.
// It handles the API call and password storage.
func updateAndSaveServicePassword(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	client api.ClientWithResponsesInterface,
	service api.Service,
	newPassword string,
	role string,
) error {
	// Call API to update password
	updateReq := api.UpdatePasswordInput{Password: newPassword}
	resp, err := client.UpdatePasswordWithResponse(ctx, *service.ProjectID, *service.ServiceID, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 204 {
		return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	// Save password locally
	if result, err := common.SavePasswordWithResult(cfg, service, newPassword, role); err != nil {
		cmd.PrintErrf("Warning: could not save password: %v\n", err)
	} else if result.Success {
		cmd.PrintErrf("%s\n", result.Message)
		cmd.PrintErrf("To view your new password, run: \n\t tiger service get %s --with-password\n", util.Deref(service.ServiceID))
	}

	return nil
}

// resetServicePassword resets the password via API. If newPassword is empty, generates one.
func resetServicePassword(ctx context.Context, cmd *cobra.Command, cfg *config.Config, client api.ClientWithResponsesInterface, service api.Service, role string, newPassword string) (string, error) {
	// Generate password if not provided
	if newPassword == "" {
		var err error
		if newPassword, err = util.GenerateSecurePassword(32); err != nil {
			return "", fmt.Errorf("failed to generate new password: %w", err)
		}
		cmd.PrintErrf("Successfully generated a new password.\n")
	}

	// Update and save password
	if err := updateAndSaveServicePassword(ctx, cmd, cfg, client, service, newPassword, role); err != nil {
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
	service api.Service,
	role string,
) (string, error) {
	cmd.PrintErr("Enter new password (leave empty to generate): ")
	newPassword, err := util.ReadPassword(ctx, cmd.InOrStdin())
	cmd.PrintErrln() // newline after password entry
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	return resetServicePassword(ctx, cmd, cfg, client, service, role, newPassword)
}
