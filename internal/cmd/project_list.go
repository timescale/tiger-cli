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

// OutputProject is a project plus the locally-derived fields the API doesn't
// return.
type OutputProject struct {
	api.Project

	// Current reports whether this is the active project, i.e. the one
	// subsequent commands operate on.
	Current bool `json:"current"`
}

// buildProjectListCmd represents the list command under project
func buildProjectListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all projects",
		Long: `List the Tiger Cloud projects you have access to.

The active project — the one subsequent commands operate on — is marked in the
output. Use 'tiger project use' to switch to another one.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			resp, err := client.GetProjectsWithResponse(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list projects: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			projects := *resp.JSON200

			return outputProjects(cmd, projects, projectID, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion())

	return cmd
}

// outputProjects formats and outputs the projects list based on the specified format
func outputProjects(cmd *cobra.Command, projects []api.Project, currentProjectID string, format string) error {
	outputProjects := prepareProjectsForOutput(projects, currentProjectID)

	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, outputProjects)
	case "yaml":
		return util.SerializeToYAML(outputWriter, outputProjects)
	case "env":
		// Not reachable through --output (outputFlag rejects it at parse time),
		// but TIGER_OUTPUT and the config file aren't validated on load.
		return fmt.Errorf("environment variable output is not supported for multiple projects")
	default: // table format (default)
		return outputProjectsTable(outputProjects, outputWriter)
	}
}

// prepareProjectsForOutput marks the active project among the ones listed
func prepareProjectsForOutput(projects []api.Project, currentProjectID string) []OutputProject {
	prepared := make([]OutputProject, len(projects))
	for i, project := range projects {
		prepared[i] = OutputProject{
			Project: project,
			Current: project.ID == currentProjectID,
		}
	}
	return prepared
}

// outputProjectsTable outputs projects in a formatted table using tablewriter
func outputProjectsTable(projects []OutputProject, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROJECT ID", "NAME", "CURRENT")

	for _, project := range projects {
		current := ""
		if project.Current {
			current = "*"
		}
		table.Append(project.ID, project.Name, current)
	}

	return table.Render()
}
