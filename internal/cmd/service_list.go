package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// serviceListCmd represents the list command under service
func buildServiceListCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List all services",
		Long:              `List all database services in the current project.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		PreRunE:           bindFlags("output"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Load config and API client
			cfg, err := common.LoadConfig(cmd.Context())
			if err != nil {
				return err
			}

			// Make API call to list services
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := cfg.Client.GetServicesWithResponse(ctx, cfg.ProjectID)
			if err != nil {
				return fmt.Errorf("failed to list services: %w", err)
			}

			statusOutput := cmd.ErrOrStderr()

			// Handle API response
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			services := *resp.JSON200

			if len(services) == 0 {
				fmt.Fprintln(statusOutput, "🏜️  No services found! Your project is looking a bit empty.")
				fmt.Fprintln(statusOutput, "🚀 Ready to get started? Create your first service with: tiger service create")
				return nil
			}

			if resp.JSON200 == nil {
				fmt.Fprintln(statusOutput, "🏜️  No services found! Your project is looking a bit empty.")
				fmt.Fprintln(statusOutput, "🚀 Ready to get started? Create your first service with: tiger service create")
				return nil
			}

			// Output services in requested format
			return outputServices(cmd, services, cfg.Output)
		},
	}

	cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "Output format (json, yaml, table)")

	return cmd
}
