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

// buildAllowListListCmd creates the allowlist list subcommand.
func buildAllowListListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all IP allow lists",
		Long:    `List every IP allow list in the current project.`,
		Example: `  # List all IP allow lists
  tiger allowlist list

  # Output as JSON
  tiger allowlist list -o json`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			resp, err := client.GetAllowListsWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list IP allow lists: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			allowLists := *resp.JSON200

			if len(allowLists) == 0 {
				cmd.PrintErrln("No IP allow lists found for this project.")
				return nil
			}

			return outputAllowLists(cmd, allowLists, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}

func outputAllowLists(cmd *cobra.Command, allowLists []api.AllowList, format string) error {
	out := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(out, allowLists)
	case "yaml":
		return util.SerializeToYAML(out, allowLists)
	case "env":
		return fmt.Errorf("environment variable output is not supported for IP allow lists")
	default:
		return outputAllowListsTable(allowLists, out)
	}
}

func outputAllowListsTable(allowLists []api.AllowList, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("ID", "DESCRIPTION", "CIDR BLOCKS", "CREATED")

	for _, allowList := range allowLists {
		if err := table.Append(
			allowList.AllowListID,
			allowList.Description,
			strings.Join(allowList.CidrBlocks, ", "),
			allowList.CreatedAt.Local().Format("2006-01-02 15:04 MST"),
		); err != nil {
			return err
		}
	}

	return table.Render()
}
