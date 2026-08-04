package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// getServiceDetailsFunc can be overridden for testing
var getServiceDetailsFunc = getServiceDetails

func buildDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations and management",
		Long:  `Database-specific operations including connection management, testing, and configuration.`,
	}

	cmd.AddCommand(buildDbConnectionStringCmd())
	cmd.AddCommand(buildDbConnectCmd())
	cmd.AddCommand(buildDbTestConnectionCmd())
	cmd.AddCommand(buildDbSavePasswordCmd())
	cmd.AddCommand(buildDbCreateCmd())
	cmd.AddCommand(buildDbSchemaCmd())

	return cmd
}

// lookupConnectionTarget looks up the target named by args, which may be a
// primary service ID or a read replica set ID. This lets a replica ID work
// anywhere a service ID does across the db connection commands.
func lookupConnectionTarget(cmd *cobra.Command, cfg *common.Config, args []string) (*common.ConnectionTarget, error) {
	service, err := getServiceDetailsFunc(cmd, cfg, args)
	if err != nil {
		return nil, err
	}

	// The API resolves both primary and read replica IDs via GetService; a read
	// replica comes back linked to its parent, whose credentials it shares.
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	return common.ResolveConnectionTarget(ctx, cfg.Client, cfg.ProjectID, service)
}

// warnReplicaPooler prints the replica pooler-fallback warning to stderr, if
// any. It is a no-op for a primary target or when there's nothing to warn.
func warnReplicaPooler(cmd *cobra.Command, target *common.ConnectionTarget, pooled bool) {
	if warning := common.ReplicaPoolerWarning(target, pooled); warning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: %s\n", warning)
	}
}

// buildConnectionDetailsForTarget builds connection details for a target,
// warning first when a replica falls back from a requested pooler.
func buildConnectionDetailsForTarget(cmd *cobra.Command, cfg *config.Config, target *common.ConnectionTarget, opts common.ConnectionDetailsOptions) (*common.ConnectionDetails, error) {
	warnReplicaPooler(cmd, target, opts.Pooled)
	return target.Details(cfg, opts)
}

// getServiceDetails is a helper that handles common service lookup logic and returns the service details
func getServiceDetails(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
	// Determine service ID
	serviceID, err := getServiceID(cfg.Config, args)
	if err != nil {
		return api.Service{}, err
	}

	cmd.SilenceUsage = true

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	service, err := common.GetService(ctx, cfg.Client, cfg.ProjectID, serviceID)
	if err != nil {
		return api.Service{}, err
	}
	return *service, nil
}
