package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildAllowListGetCmd creates the allowlist get subcommand.
func buildAllowListGetCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <allow-list-id>",
		Aliases: []string{"describe", "show"},
		Short:   "Show detailed information about an IP allow list",
		Long:    `Show the details of a single IP allow list.`,
		Example: `  # Get an IP allow list
  tiger allowlist get 1234567890

  # Get IP allow list details in JSON format
  tiger allowlist get 1234567890 --output json`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			resp, err := client.GetAllowListWithResponse(cmd.Context(), projectID, args[0])
			if err != nil {
				return fmt.Errorf("failed to get IP allow list: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			return outputAllowList(cmd, *resp.JSON200, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}
