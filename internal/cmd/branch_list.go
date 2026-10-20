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

			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}
			service, err := resolveService(cmd.Context(), client, projectID, serviceRef)
			if err != nil {
				return err
			}

			resp, err := client.GetBranchesWithResponse(cmd.Context(), projectID, service.ServiceID, nil)
			if err != nil {
				return fmt.Errorf("failed to list branches: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			return outputBranches(cmd, cfg, *service, *resp.JSON200)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}

// outputBranches outputs the branches of service in the configured output
// format.
func outputBranches(cmd *cobra.Command, cfg *config.Config, service api.Service, branches []api.Branch) error {
	opts := branchOutputOptions{ServiceName: service.Name}
	prepared := make([]OutputBranch, len(branches))
	for i, branch := range branches {
		prepared[i] = prepareBranchForOutput(cmd, cfg, branch, opts)
	}

	outputWriter := cmd.OutOrStdout()
	switch strings.ToLower(cfg.Output) {
	case "json":
		return util.SerializeToJSON(outputWriter, prepared)
	case "yaml":
		return util.SerializeToYAML(outputWriter, prepared)
	default: // table format (default)
		if len(prepared) == 0 {
			cmd.PrintErrf("No branches found for service %s.\n", serviceLabel(service))
			return nil
		}
		return outputBranchesTable(outputWriter, prepared)
	}
}

// outputBranchesTable outputs branches in a formatted table.
func outputBranchesTable(output io.Writer, branches []OutputBranch) error {
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
