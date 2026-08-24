package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// fetchProjects lists the projects the logged-in user can access.
func fetchProjects(ctx context.Context, client api.ClientWithResponsesInterface) ([]api.Project, error) {
	resp, err := client.GetProjectsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	return *resp.JSON200, nil
}

// requireProjectAccess verifies that projectID is one of projects. The
// rejected ID goes to stderr only, not into the error text that analytics
// records verbatim.
func requireProjectAccess(cmd *cobra.Command, projects []api.Project, projectID string) error {
	if slices.ContainsFunc(projects, func(p api.Project) bool { return p.ID == projectID }) {
		return nil
	}
	cmd.PrintErrf("Project %s is not among your accessible projects\n", projectID)
	return common.ExitWithCode(common.ExitInvalidParameters, errors.New("no access to the requested project"))
}

// clearStaleDefaultService removes the service_id config value after the
// active project changed: a default service belongs to the project it was set
// in. Used by `tiger project` and `tiger auth login`.
func clearStaleDefaultService(cmd *cobra.Command, cfg *config.Config) {
	if cfg.ServiceID == "" {
		return
	}
	if err := cfg.Unset("service_id"); err != nil {
		cmd.PrintErrf("Warning: failed to clear default service: %v\n", err)
		return
	}
	// Unset reloads cfg in place; a value that survives comes from
	// --service-id or TIGER_SERVICE_ID, which Unset can't clear.
	if cfg.ServiceID != "" {
		cmd.PrintErrln("Warning: the default service from --service-id/TIGER_SERVICE_ID belongs to the previous project and is still in effect")
		return
	}
	cmd.PrintErrln("Cleared default service (config key service_id): it belonged to the previous project")
}
