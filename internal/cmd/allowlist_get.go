package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
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

func outputAllowList(cmd *cobra.Command, allowList api.AllowList, format string) error {
	out := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(out, allowList)
	case "yaml":
		return util.SerializeToYAML(out, allowList)
	case "env":
		return fmt.Errorf("environment variable output is not supported for IP allow lists")
	default:
		return outputAllowListTable(allowList, out)
	}
}

func outputAllowListTable(allowList api.AllowList, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROPERTY", "VALUE")
	if err := table.Append("ID", allowList.AllowListID); err != nil {
		return err
	}
	if err := table.Append("Description", allowList.Description); err != nil {
		return err
	}
	if err := table.Append("CIDR Blocks", strings.Join(allowList.CidrBlocks, ", ")); err != nil {
		return err
	}
	if err := table.Append("Created", allowList.CreatedAt.Local().Format("2006-01-02 15:04 MST")); err != nil {
		return err
	}
	return table.Render()
}
