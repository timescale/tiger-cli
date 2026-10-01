package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildAllowListCreateCmd creates the allowlist create subcommand.
func buildAllowListCreateCmd(app *common.App) *cobra.Command {
	var (
		description string
		cidrBlocks  []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an IP allow list",
		Long: `Create an IP allow list that restricts which IP ranges can reach a service.

An IP allow list carries no effect until it is attached to a service with
'tiger service allowlist attach'. The number of CIDR blocks per list and the
number of lists per project are plan-dependent.`,
		Example: `  # Create an IP allow list for the office network
  tiger allowlist create --description "Office network" --cidr 203.0.113.0/24

  # Create one covering multiple ranges
  tiger allowlist create --description "VPN ranges" --cidr 203.0.113.0/24 --cidr 198.51.100.0/24`,
		Args:              cobra.NoArgs,
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

			resp, err := client.CreateAllowListWithResponse(cmd.Context(), projectID, api.AllowListCreate{
				Description: description,
				CidrBlocks:  cidrBlocks,
			})
			if err != nil {
				return fmt.Errorf("failed to create IP allow list: %w", err)
			}

			if resp.StatusCode() != http.StatusCreated {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON201 == nil {
				return fmt.Errorf("empty response from API")
			}

			cmd.PrintErrf("IP allow list '%s' created.\n", description)

			return outputAllowList(cmd, *resp.JSON201, cfg.Output)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Human-readable label for the IP allow list")
	cmd.Flags().StringArrayVar(&cidrBlocks, "cidr", nil, "A CIDR block to permit (repeatable)")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	markFlagRequired(cmd, "description")
	markFlagRequired(cmd, "cidr")

	return cmd
}
