package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// getServiceDetailsFunc can be overridden for testing
var getServiceDetailsFunc = getServiceDetails

func buildDbCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations and management",
		Long:  `Database-specific operations including connection management, testing, and configuration.`,
	}

	cmd.AddCommand(buildDbConnectionStringCmd(app))
	cmd.AddCommand(buildDbConnectCmd(app))
	cmd.AddCommand(buildDbTestConnectionCmd(app))
	cmd.AddCommand(buildDbSavePasswordCmd(app))
	cmd.AddCommand(buildDbCreateCmd(app))
	cmd.AddCommand(buildDbSchemaCmd(app))

	return cmd
}

// lookupConnectionTarget looks up the target named by args, which may be a
// primary service ID or a read replica set ID. This lets a replica ID work
// anywhere a service ID does across the db connection commands.
func lookupConnectionTarget(cmd *cobra.Command, app *common.App, args []string) (*common.ConnectionTarget, error) {
	service, err := getServiceDetailsFunc(cmd, app, args)
	if err != nil {
		return nil, err
	}

	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	// The API resolves both primary and read replica IDs via GetService; a read
	// replica comes back linked to its parent, whose credentials it shares.
	return common.ResolveConnectionTarget(cmd.Context(), client, projectID, service)
}

// warnReplicaPooler prints the replica pooler-fallback warning to stderr, if
// any. It is a no-op for a primary target or when there's nothing to warn.
func warnReplicaPooler(cmd *cobra.Command, target *common.ConnectionTarget, pooled bool) {
	if warning := common.ReplicaPoolerWarning(target, pooled); warning != "" {
		cmd.PrintErrf("⚠️  Warning: %s\n", warning)
	}
}

// buildConnectionDetailsForTarget builds connection details for a target,
// warning first when a replica falls back from a requested pooler.
func buildConnectionDetailsForTarget(cmd *cobra.Command, cfg *config.Config, target *common.ConnectionTarget, opts common.ConnectionDetailsOptions) (*common.ConnectionDetails, error) {
	warnReplicaPooler(cmd, target, opts.Pooled)
	return target.Details(cfg, opts)
}

// getServiceDetails is a helper that handles common service lookup logic and returns the service details
func getServiceDetails(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
	cfg, client, projectID, err := app.GetAll()
	if err != nil {
		return api.Service{}, err
	}

	// Determine service ID
	serviceID, err := getServiceID(cfg, args)
	if err != nil {
		return api.Service{}, err
	}

	service, err := common.GetService(cmd.Context(), client, projectID, serviceID)
	if err != nil {
		return api.Service{}, err
	}
	return *service, nil
}
