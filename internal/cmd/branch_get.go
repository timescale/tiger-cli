package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchGetCmd creates the get subcommand
func buildBranchGetCmd(app *common.App) *cobra.Command {
	var withPassword bool

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
  tiger branch get br-12345 --output json

  # Include the stored password
  tiger branch get br-12345 --with-password`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchRefCompletion(app),
		SilenceUsage:      true,
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

			// The branch carries only its parent's ID, so the name takes a
			// second fetch. The rest of the output doesn't depend on it.
			opts := branchOutputOptions{
				WithPassword:    withPassword,
				RequirePassword: true,
			}
			if parent, err := common.JSON200(client.GetServiceWithResponse(cmd.Context(), projectID, branch.ServiceID)); err != nil {
				cmd.PrintErrf("Warning: failed to get parent service: %v\n", err)
			} else {
				opts.ServiceName = parent.Name
			}

			return outputBranch(cmd, cfg, *branch, opts)
		},
	}

	cmd.Flags().BoolVar(&withPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")
	registerFlagCompletion(cmd, "output", outputCompletion("env"))

	return cmd
}
