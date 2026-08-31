package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

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
	cmd.AddCommand(buildDbQueryCmd(app))

	return cmd
}

// handleDatabaseError turns the readiness sentinels into guidance naming the
// command that resolves them. Every other error passes through unchanged.
func handleDatabaseError(err error, serviceID string) error {
	switch {
	case errors.Is(err, common.ErrPaused):
		return fmt.Errorf("%w — start it with 'tiger service start %s'", common.ErrPaused, serviceID)
	case errors.Is(err, common.ErrNotReady):
		return fmt.Errorf("%w — check its status with 'tiger service get %s' and try again", common.ErrNotReady, serviceID)
	}
	return err
}

// warnReplicaPooler prints the replica pooler-fallback warning to stderr, if
// any. It is a no-op for a primary target or when there's nothing to warn.
func warnReplicaPooler(cmd *cobra.Command, target *common.ConnectionTarget, pooled bool) {
	if warning := common.ReplicaPoolerWarning(target, pooled); warning != "" {
		cmd.PrintErrf("⚠️  Warning: %s\n", warning)
	}
}
