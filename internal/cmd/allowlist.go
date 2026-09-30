package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildAllowListCmd creates the allowlist group command.
func buildAllowListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "allowlist",
		Aliases: []string{"allowlists"},
		Short:   "Manage IP allow lists",
		Long:    `Manage the IP allow lists that restrict which IP ranges can reach your services.`,
	}

	cmd.AddCommand(buildAllowListListCmd(app))
	cmd.AddCommand(buildAllowListGetCmd(app))
	cmd.AddCommand(buildAllowListCreateCmd(app))
	cmd.AddCommand(buildAllowListUpdateCmd(app))
	cmd.AddCommand(buildAllowListDeleteCmd(app))

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
