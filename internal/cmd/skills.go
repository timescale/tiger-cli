package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildSkillsCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage agent skills for AI coding agents",
		Long: `Manage agent skills for AI coding agents.

Skills give AI coding agents curated knowledge and best practices for working
with PostgreSQL, TimescaleDB, and Tiger Cloud.`,
	}

	cmd.AddCommand(buildSkillsInstallCmd(app))

	return cmd
}
