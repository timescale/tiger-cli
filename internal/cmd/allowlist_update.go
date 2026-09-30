package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildAllowListUpdateCmd creates the allowlist update subcommand.
func buildAllowListUpdateCmd(app *common.App) *cobra.Command {
	var (
		description string
		cidrBlocks  []string
	)

	cmd := &cobra.Command{
		Use:   "update <allow-list-id>",
		Short: "Update an IP allow list",
		Long: `Update an IP allow list in place.

Provide --description and/or --cidr to change them; at least one is
required. Omitting one leaves it unchanged. --cidr replaces the whole set of
CIDR blocks, not just the ones changing; to remove every CIDR block, delete
the IP allow list instead.

Every service the IP allow list is attached to picks up the new CIDR blocks;
there is no need to detach and reattach.`,
		Example: `  # Rename an IP allow list
  tiger allowlist update 1234567890 --description "New name"

  # Replace the CIDR blocks
  tiger allowlist update 1234567890 --cidr 203.0.113.0/24 --cidr 198.51.100.0/24`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			if cfg.ReadOnly.BlocksAll() {
				return common.ErrReadOnly
			}

			descriptionChanged := cmd.Flags().Changed("description")
			cidrChanged := cmd.Flags().Changed("cidr")
			if !descriptionChanged && !cidrChanged {
				return common.ExitWithCode(common.ExitInvalidParameters, errors.New("at least one of --description or --cidr is required"))
			}

			update := api.AllowListUpdate{}
			if descriptionChanged {
				update.Description = &description
			}
			if cidrChanged {
				update.CidrBlocks = &cidrBlocks
			}

			resp, err := client.UpdateAllowListWithResponse(cmd.Context(), projectID, args[0], update)
			if err != nil {
				return fmt.Errorf("failed to update IP allow list: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			cmd.PrintErrf("IP allow list '%s' updated.\n", args[0])

			return outputAllowList(cmd, *resp.JSON200, cfg.Output)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "New human-readable label for the IP allow list")
	cmd.Flags().StringArrayVar(&cidrBlocks, "cidr", nil, "Replacement CIDR blocks to permit (repeatable)")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}
