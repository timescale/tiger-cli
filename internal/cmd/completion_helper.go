package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/mcp"
)

// withAppLoad wraps a completion function, loading the config and API client
// before invoking it. The App is only loaded automatically for wrapped commands
// (see wrapCommands), not for the __complete command that drives live tab
// completion — so completions that don't need the config or client (static
// lists, subcommand and flag names) stay clear of the config file, the system
// keyring, and the network. Completion functions that do need them must be
// wrapped with this helper.
func withAppLoad(app *common.App, fn cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		app.SetFlags(cmd.Flags())
		if _, _, _, err := app.Load(cmd.Context()); err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return fn(cmd, args, toComplete)
	}
}

func serviceIDCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Service ID is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		services, err := listServices(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		results := make([]string, 0, len(services))
		for _, service := range services {
			if strings.HasPrefix(service.ServiceID, toComplete) {
				results = append(results, cobra.CompletionWithDesc(service.ServiceID, service.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

func listServices(cmd *cobra.Command, app *common.App) ([]api.Service, error) {
	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	// Make API call to list services
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := client.GetServicesWithResponse(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil || len(*resp.JSON200) == 0 {
		return []api.Service{}, nil
	}

	return *resp.JSON200, nil
}

func configOptionCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// Completing the key
		return filterCompletionsByPrefix(config.ValidConfigOptions(), toComplete), cobra.ShellCompDirectiveNoFileComp
	case 1:
		// Completing the value, based on the key already typed
		return filterCompletionsByPrefix(config.ValidConfigOptionValues(args[0]), toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// validEnvironmentTags are the accepted values for --environment on `service
// create` and `service fork`.
var validEnvironmentTags = []string{"DEV", "PROD"}

var environmentCompletion = cobra.FixedCompletions(validEnvironmentTags, cobra.ShellCompDirectiveNoFileComp)

// addonsCompletion completes --addons on `service create`, drawn from the
// same list used to validate it (common.ValidateAddons).
var addonsCompletion = cobra.FixedCompletions(append(common.ValidAddons(), common.AddonNone), cobra.ShellCompDirectiveNoFileComp)

// passwordStorageCompletion completes the global --password-storage flag,
// drawn from the same list used to validate `tiger config set password_storage`.
var passwordStorageCompletion = cobra.FixedCompletions(config.ValidPasswordStorageOptions(), cobra.ShellCompDirectiveNoFileComp)

// metricsSeriesRoleCompletion completes --role on `service metrics series`.
var metricsSeriesRoleCompletion = cobra.FixedCompletions([]string{"PRIMARY", "REPLICA"}, cobra.ShellCompDirectiveNoFileComp)

// metricsSeriesFnCompletion completes --fn on `service metrics series`, drawn
// from the generated MetricsSeriesRequestFn enum.
var metricsSeriesFnCompletion = cobra.FixedCompletions([]string{
	string(api.MetricsSeriesRequestFnAVG),
	string(api.MetricsSeriesRequestFnCOUNT),
	string(api.MetricsSeriesRequestFnINCREASE),
	string(api.MetricsSeriesRequestFnLAST),
	string(api.MetricsSeriesRequestFnMAX),
	string(api.MetricsSeriesRequestFnMIN),
	string(api.MetricsSeriesRequestFnP50),
	string(api.MetricsSeriesRequestFnP90),
	string(api.MetricsSeriesRequestFnP99),
	string(api.MetricsSeriesRequestFnRATE),
	string(api.MetricsSeriesRequestFnSUM),
}, cobra.ShellCompDirectiveNoFileComp)

// outputCompletion returns a completion func for --output/-o flags. extra
// lists any command-specific formats beyond the universal json/yaml/table
// (e.g. "env" on `service create/get/fork`, "bare" on `version`).
func outputCompletion(extra ...string) cobra.CompletionFunc {
	return cobra.FixedCompletions(config.ValidOutputFormats(extra...), cobra.ShellCompDirectiveNoFileComp)
}

// cpuCompletion and memoryCompletion complete the --cpu and --memory flags,
// drawn from the same allowed configurations used to validate the pair
// (common.ValidateAndNormalizeCPUMemory).
func cpuCompletion(configs common.CPUMemoryConfigs) cobra.CompletionFunc {
	values := make([]string, 0, len(configs))
	for _, c := range configs {
		values = append(values, *c.CPUMillisString())
	}
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp)
}

func memoryCompletion(configs common.CPUMemoryConfigs) cobra.CompletionFunc {
	values := make([]string, 0, len(configs))
	for _, c := range configs {
		values = append(values, *c.MemoryGBsString())
	}
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp)
}

// mcpGetCompletion provides custom completions for the get command
func mcpGetCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Capability name is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Create MCP server to get capabilities
		server, err := mcp.NewServer(cmd.Context(), app, nil)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer server.Close()

		capabilities, err := server.ListCapabilities(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Close the MCP server when finished
		if err := server.Close(); err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return filterCompletionsByPrefix(capabilities.Names(), toComplete), cobra.ShellCompDirectiveNoFileComp
	})
}

// filterCompletionsByPrefix filters a slice of strings to only include items
// that start with the given prefix. This is used by shell completion functions
// to narrow down suggestions based on what the user has typed so far.
func filterCompletionsByPrefix(items []string, prefix string) []string {
	var filtered []string
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
