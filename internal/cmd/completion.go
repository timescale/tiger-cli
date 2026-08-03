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
)

func serviceIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Service ID is always first positional argument
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	services, err := listServices(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	results := make([]string, 0, len(services))
	for _, service := range services {
		if service.ServiceId != nil && strings.HasPrefix(*service.ServiceId, toComplete) {
			results = append(results, cobra.CompletionWithDesc(*service.ServiceId, *service.Name))
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
}

func listServices(cmd *cobra.Command) ([]api.Service, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(cmd.Context())
	if err != nil {
		return nil, err
	}

	// Make API call to list services
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetServicesWithResponse(ctx, cfg.ProjectID)
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
