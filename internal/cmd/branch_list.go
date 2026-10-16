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
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildBranchListCmd creates the list subcommand
func buildBranchListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [service-name-or-id]",
		Aliases: []string{"ls"},
		Short:   "List the branches of a service",
		Long: `List the branches of a database service.

The service can be given by ID or name as an argument, or will use the default
service from your configuration.`,
		Example: `  # List the default service's branches
  tiger branch list

  # List a specific service's branches
  tiger branch list svc-12345`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// TODO: when the default is a branch, list its parent's branches.
			// For now the default is assumed to be a service, and a branch
			// fails to resolve as one.
			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}
			service, err := resolveService(cmd.Context(), client, projectID, serviceRef)
			if err != nil {
				return err
			}

			// TODO: the API lists every branch in the project, so the service's
			// are picked out here. Filter server-side if the API gains a
			// service filter.
			resp, err := client.GetBranchesWithResponse(cmd.Context(), projectID, nil)
			if err != nil {
				return fmt.Errorf("failed to list branches: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			var branches []api.Branch
			for _, branch := range *resp.JSON200 {
				if branch.ServiceID == service.ServiceID {
					branches = append(branches, branch)
				}
			}

			if len(branches) == 0 {
				cmd.PrintErrf("No branches found for service %s.\n", serviceLabel(*service))
				return nil
			}

			return outputBranches(cmd, cfg, branches, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}

// outputBranches formats and outputs a list of branches in the given format.
func outputBranches(cmd *cobra.Command, cfg *config.Config, branches []api.Branch, format string) error {
	prepared := make([]OutputBranch, len(branches))
	for i, branch := range branches {
		prepared[i] = prepareBranchForOutput(cmd, cfg, branch)
	}

	outputWriter := cmd.OutOrStdout()
	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, prepared)
	case "yaml":
		return util.SerializeToYAML(outputWriter, prepared)
	default: // table format (default)
		return outputBranchesTable(prepared, outputWriter)
	}
}

// outputBranchesTable outputs branches in a formatted table.
func outputBranchesTable(branches []OutputBranch, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("BRANCH ID", "NAME", "STATUS", "CREATED")

	for _, branch := range branches {
		table.Append(
			branch.ID,
			branch.Name,
			string(branch.Status),
			branch.Created.Format("2006-01-02 15:04"),
		)
	}

	return table.Render()
}
