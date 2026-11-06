package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations and management",
		Long:  `Database-specific operations including connection management, testing, and configuration.`,
	}

	cmd.AddCommand(buildDbURICmd(app))
	cmd.AddCommand(buildDbPsqlCmd(app))
	cmd.AddCommand(buildDbPingCmd(app))
	cmd.AddCommand(buildDbSavePasswordCmd(app))
	cmd.AddCommand(buildDbCreateCmd(app))
	cmd.AddCommand(buildDbSchemaCmd(app))
	cmd.AddCommand(buildDbExtensionsCmd(app))
	cmd.AddCommand(buildDbQueryCmd(app))

	return cmd
}

// branchRefHelp is the paragraph a db command's help gains for branches, saying
// what passing one does. Branches are a preview, so it is empty without the
// experimental gate.
func branchRefHelp(experimental bool, text string) string {
	if !experimental {
		return ""
	}
	return "\n\n" + text
}

// handleDatabaseError adds guidance to the failures whose fix isn't evident from
// the message: the readiness sentinels, and a Postgres authentication failure,
// which otherwise arrives as a bare pgx error saying nothing about the password
// it was rejected for. Every other error passes through unchanged.
func handleDatabaseError(err error, target *common.ConnectionTarget) error {
	// A branch is managed through `tiger branch`, everything else through
	// `tiger service`. The group also names the target in the readiness
	// errors, whose sentinels always say "service".
	group := "service"
	if target.Kind == common.TargetBranch {
		group = "branch"
	}

	switch {
	case errors.Is(err, common.ErrPaused):
		return fmt.Errorf("%s is paused — start it with 'tiger %s start %s'", group, group, target.ID)
	case errors.Is(err, common.ErrNotReady):
		return fmt.Errorf("%s is not ready — check its status with 'tiger %s get %s' and try again", group, group, target.ID)
	case isPostgresAuthenticationError(err):
		// A read replica shares its primary's credentials, so the password
		// belongs to the primary rather than the replica connected to.
		serviceID := target.Credentials.ID
		return fmt.Errorf("%w\n\nThe stored password is missing or invalid. Save the current one with 'tiger db save-password %s', or reset it with 'tiger %s update-password %s'",
			err, serviceID, group, serviceID)
	}
	return err
}

// resolveConnectionTarget resolves the ref a db command was given to what it
// connects to: a service, a read replica set, or (behind the experimental
// gate) a branch, by ID or name.
func resolveConnectionTarget(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, ref serviceRef, experimental bool) (*common.ConnectionTarget, error) {
	hint := "Run 'tiger service list' to find the ID you want"
	if experimental {
		hint = "Run 'tiger service list' or 'tiger branch list' to find the ID you want"
	}

	target, err := common.ResolveConnectionTarget(ctx, client, projectID, ref.ref, experimental)
	if err != nil {
		return nil, withAmbiguousRefHint(err, hint)
	}
	if err := checkDefaultIsID(target.ID, ref); err != nil {
		return nil, err
	}
	return target, nil
}

// isPostgresAuthenticationError checks if the error is a PostgreSQL authentication failure
func isPostgresAuthenticationError(err error) bool {
	// Check for PostgreSQL error code 28P01 (invalid_password) or 28000 (invalid_authorization_specification)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "28P01" || pgErr.Code == "28000"
	}
	return false
}

// warnReplicaPooler prints the replica pooler-fallback warning to stderr, if
// any. It is a no-op for a primary target or when there's nothing to warn.
func warnReplicaPooler(cmd *cobra.Command, target *common.ConnectionTarget, pooled bool) {
	if warning := common.ReplicaPoolerWarning(target, pooled); warning != "" {
		cmd.PrintErrf("Warning: %s\n", warning)
	}
}
