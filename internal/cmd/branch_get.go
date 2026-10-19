package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchGetCmd creates the get subcommand
func buildBranchGetCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <name-or-id>",
		Aliases: []string{"describe", "show"},
		Short:   "Show detailed information about a branch",
		Long: `Show detailed information about a branch, including its parent service, status,
compute, and connection details.

The branch can be given by ID or name.`,
		Example: `  # Get a branch's details
  tiger branch get br-12345

  # Get branch details in JSON format
  tiger branch get br-12345 --output json`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			branchRef, err := getBranchRef(args)
			if err != nil {
				return err
			}

			branch, err := resolveBranch(cmd.Context(), client, projectID, branchRef)
			if err != nil {
				return err
			}

			return outputBranch(cmd, cfg, *branch, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")
	registerFlagCompletion(cmd, "output", outputCompletion("env"))

	return cmd
}
