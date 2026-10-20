package cmd

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/mcp"
)

// dirCompletion completes directory names, and fileCompletion file names —
// filtered to the given extensions, or any file when called with none.
// Completion defaults to offering nothing at all (see buildRootCmd), so a flag
// that takes a path has to ask for these.
var dirCompletion = cobra.FixedCompletions(nil, cobra.ShellCompDirectiveFilterDirs)

func fileCompletion(extensions ...string) cobra.CompletionFunc {
	if len(extensions) == 0 {
		return cobra.FixedCompletions(nil, cobra.ShellCompDirectiveDefault)
	}
	return cobra.FixedCompletions(extensions, cobra.ShellCompDirectiveFilterFileExt)
}

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

func serviceRefCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// The service ref is always the first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		services, err := listServices(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Suggest each service at most once: prefer name, fall back to ID
		results := make([]string, 0, len(services))
		for _, service := range services {
			if strings.HasPrefix(service.Name, toComplete) {
				results = append(results, cobra.CompletionWithDesc(service.Name, service.ServiceID))
			} else if strings.HasPrefix(service.ServiceID, toComplete) {
				results = append(results, cobra.CompletionWithDesc(service.ServiceID, service.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

// serviceIDFlagCompletion completes the global --service-id flag. Unlike the
// positional completion, it offers only IDs, since a default service must be
// an ID (see serviceRef), and it offers them no matter how many arguments have
// been typed — the flag is the same wherever it appears in the command line.
func serviceIDFlagCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	resp, err := client.GetServicesWithResponse(cmd.Context(), projectID, nil)
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

// branchRefCompletion completes the branch argument of the branch commands.
// Like serviceRefCompletion, it offers each branch at most once, by name,
// falling back to its ID when what is typed matches the ID instead.
func branchRefCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// The branch ref is always the first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		client, projectID, err := app.GetClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		resp, err := client.GetProjectBranchesWithResponse(cmd.Context(), projectID)
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		branches := *resp.JSON200
		results := make([]string, 0, len(branches))
		for _, branch := range branches {
			if strings.HasPrefix(branch.Name, toComplete) {
				results = append(results, cobra.CompletionWithDesc(branch.Name, branch.ID))
			} else if strings.HasPrefix(branch.ID, toComplete) {
				results = append(results, cobra.CompletionWithDesc(branch.ID, branch.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

func projectIDCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Project ID is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		projects, err := listProjects(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		results := make([]string, 0, len(projects))
		for _, project := range projects {
			if strings.HasPrefix(project.ID, toComplete) {
				results = append(results, cobra.CompletionWithDesc(project.ID, project.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

func listProjects(cmd *cobra.Command, app *common.App) ([]api.Project, error) {
	client, _, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.GetProjectsWithResponse(cmd.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return []api.Project{}, nil
	}

	return *resp.JSON200, nil
}

// configKeyCompletion completes the <key> argument of `tiger config unset`,
// which takes no value argument.
func configKeyCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletionsByPrefix(config.PublicConfigOptions(), toComplete), cobra.ShellCompDirectiveNoFileComp
}

// configKeyValueCompletion completes the <key> and <value> arguments of
// `tiger config set`.
func configKeyValueCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// Completing the key
		return filterCompletionsByPrefix(config.PublicConfigOptions(), toComplete), cobra.ShellCompDirectiveNoFileComp
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

// platformCompletion completes --platform on `service create`, drawn from the
// same list used to validate it (common.ValidPlatforms).
var platformCompletion = cobra.FixedCompletions(common.ValidPlatforms(), cobra.ShellCompDirectiveNoFileComp)

// addonsCompletion completes --addons on `service create`, drawn from the
// same list used to validate it (common.ValidateAddons).
var addonsCompletion = cobra.FixedCompletions(append(common.ValidAddons(), common.AddonNone), cobra.ShellCompDirectiveNoFileComp)

// passwordStorageCompletion completes the global --password-storage flag,
// drawn from the same list used to validate `tiger config set password_storage`.
var passwordStorageCompletion = cobra.FixedCompletions(config.ValidPasswordStorageOptions(), cobra.ShellCompDirectiveNoFileComp)

// metricNameCompletion completes --metric on `service metrics details` and
// `service metrics series`, drawn from the target service's available metric
// series (the same list `service metrics available` returns).
func metricNameCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, client, projectID, err := app.GetAll()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		serviceRef, err := getServiceRef(cmd, cfg, args)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		service, err := resolveService(cmd.Context(), client, projectID, serviceRef)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		resp, err := client.GetServiceMetricsAvailableSeriesWithResponse(cmd.Context(), projectID, service.ServiceID)
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return filterCompletionsByPrefix(*resp.JSON200, toComplete), cobra.ShellCompDirectiveNoFileComp
	})
}

// metricsSeriesRoleCompletion completes --role on `service metrics series`.
var metricsSeriesRoleCompletion = cobra.FixedCompletions([]string{"PRIMARY", "REPLICA"}, cobra.ShellCompDirectiveNoFileComp)

// metricsSeriesFnCompletion completes --fn on `service metrics series`, drawn
// from the generated MetricsAggFn enum.
var metricsSeriesFnCompletion = cobra.FixedCompletions([]string{
	string(api.MetricsAggFnAVG),
	string(api.MetricsAggFnCOUNT),
	string(api.MetricsAggFnINCREASE),
	string(api.MetricsAggFnLAST),
	string(api.MetricsAggFnMAX),
	string(api.MetricsAggFnMAXTOTAL),
	string(api.MetricsAggFnMIN),
	string(api.MetricsAggFnMINTOTAL),
	string(api.MetricsAggFnP50),
	string(api.MetricsAggFnP90),
	string(api.MetricsAggFnP99),
	string(api.MetricsAggFnRATE),
	string(api.MetricsAggFnSUM),
}, cobra.ShellCompDirectiveNoFileComp)

// outputCompletion returns a completion func for --output/-o flags. extra
// lists any command-specific formats beyond the universal json/yaml/table
// (e.g. "env" on `service create/get/fork`, "bare" on `version`).
func outputCompletion(extra ...string) cobra.CompletionFunc {
	return cobra.FixedCompletions(config.ValidOutputFormats(extra...), cobra.ShellCompDirectiveNoFileComp)
}

// cpuCompletion and memoryCompletion complete the --cpu and --memory flags,
// drawn from the same allowed configurations used to validate the pair
// (common.AllowedCPUMemoryConfigs). shared includes shared/shared, which a
// service can't be resized onto, and experimental includes the fluid platform's
// configurations, which are a preview.
func cpuCompletion(shared, experimental bool) cobra.CompletionFunc {
	return sizeCompletion(serviceConfigs(shared, experimental), cpuMillis)
}

func memoryCompletion(shared, experimental bool) cobra.CompletionFunc {
	return sizeCompletion(serviceConfigs(shared, experimental), memoryGBs)
}

// branchCPUCompletion and branchMemoryCompletion complete the --cpu and
// --memory flags of the branch commands, which only take the configurations
// in common.BranchCPUMemoryConfigs.
var (
	branchCPUCompletion    = sizeCompletion(branchConfigs, cpuMillis)
	branchMemoryCompletion = sizeCompletion(branchConfigs, memoryGBs)
)

func cpuMillis(c common.CPUMemoryConfig) int { return c.CPUMillis }
func memoryGBs(c common.CPUMemoryConfig) int { return c.MemoryGBs }

// serviceConfigs returns the configurations the service commands' sizes are
// drawn from (see completionCPUMemoryConfigs).
func serviceConfigs(shared, experimental bool) func(*cobra.Command) common.CPUMemoryConfigs {
	return func(cmd *cobra.Command) common.CPUMemoryConfigs {
		return completionCPUMemoryConfigs(cmd, shared, experimental)
	}
}

func branchConfigs(*cobra.Command) common.CPUMemoryConfigs {
	return common.BranchCPUMemoryConfigs()
}

// sizeCompletion completes the sizes size reads off the allowed
// configurations, each once, in ascending numeric order with "shared" last.
// The configurations can come from more than one platform, so the order is
// imposed here, and the shell is told to keep it rather than sort the values
// as strings.
func sizeCompletion(configsFor func(*cobra.Command) common.CPUMemoryConfigs, size func(common.CPUMemoryConfig) int) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		configs := configsFor(cmd)

		sizes := make([]int, 0, len(configs))
		hasShared := false
		for _, c := range configs {
			if c.Shared {
				hasShared = true
			} else {
				sizes = append(sizes, size(c))
			}
		}
		slices.Sort(sizes)
		sizes = slices.Compact(sizes)

		values := make([]string, 0, len(sizes)+1)
		for _, s := range sizes {
			values = append(values, strconv.Itoa(s))
		}
		if hasShared {
			values = append(values, "shared")
		}
		return values, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
	}
}

// completionCPUMemoryConfigs returns the configurations sizeCompletion draws
// from. On a command with a --platform flag (`service create` behind the
// experimental gate) that's already been given, they narrow to that platform's;
// otherwise every allowed platform's are offered, since --platform may still
// come later on the line.
func completionCPUMemoryConfigs(cmd *cobra.Command, shared, experimental bool) common.CPUMemoryConfigs {
	if platform := cmd.Flags().Lookup("platform"); platform != nil && platform.Changed {
		switch strings.ToLower(platform.Value.String()) {
		case common.PlatformStandard:
			configs := common.StandardCPUMemoryConfigs()
			if !shared {
				return configs.WithoutShared()
			}
			return configs
		case common.PlatformFluid:
			return common.FluidCPUMemoryConfigs()
		}
	}
	return common.AllowedCPUMemoryConfigs(shared, experimental)
}

// The widest --iops range across every plan, platform, and CPU/memory
// allocation, in the steps it's configured in.
const (
	minCompletionIOPS = 3000
	maxCompletionIOPS = 256000
	iopsIncrement     = 1000
)

// iopsCompletion completes --iops on `service create`, `fork`, and `resize`,
// and on `branch create` and `resize`, with every value in the widest range. Which of them a service accepts
// depends on its plan, platform, and CPU/memory allocation, which is left to
// the API to validate.
var iopsCompletion = func() cobra.CompletionFunc {
	values := make([]string, 0, (maxCompletionIOPS-minCompletionIOPS)/iopsIncrement+1)
	for v := minCompletionIOPS; v <= maxCompletionIOPS; v += iopsIncrement {
		values = append(values, strconv.Itoa(v))
	}
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveKeepOrder)
}()

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
