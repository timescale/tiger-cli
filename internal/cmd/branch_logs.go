package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchLogsCmd creates the logs subcommand
func buildBranchLogsCmd(app *common.App) *cobra.Command {
	var tail int
	var since time.Time
	var until time.Time

	cmd := &cobra.Command{
		Use:     "logs <name-or-id>",
		Aliases: []string{"log"},
		Short:   "View logs for a branch",
		Long: `View logs for a branch.

Fetches and displays logs from the specified branch. By default, shows the last
100 log entries. Supports filtering by time range.

The branch can be given by ID or name.`,
		Example: `  # View last 100 logs for a branch (default behavior)
  tiger branch logs br-12345

  # View logs within a time range
  tiger branch logs br-12345 --since "2024-01-15T09:00:00Z" --until "2024-01-15T10:00:00Z"

  # View last 50 lines
  tiger branch logs br-12345 --tail 50

  # View last 1000 lines
  tiger branch logs br-12345 --tail 1000`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			branchRef, err := getBranchRef(args)
			if err != nil {
				return err
			}

			branch, err := resolveBranch(cmd.Context(), client, projectID, branchRef)
			if err != nil {
				return err
			}

			var sincePtr *time.Time
			if !since.IsZero() {
				sincePtr = &since
			}

			var untilPtr *time.Time
			if !until.IsZero() {
				untilPtr = &until
			}

			logs, err := common.FetchBranchLogs(cmd.Context(), common.FetchBranchLogsArgs{
				Client:    client,
				ProjectID: projectID,
				ServiceID: branch.ServiceID,
				BranchID:  branch.ID,
				Tail:      tail,
				Since:     sincePtr,
				Until:     untilPtr,
			})
			if err != nil {
				return err
			}

			return outputLogs(cmd, cfg, logs)
		},
	}

	cmd.Flags().IntVar(&tail, "tail", 100, "Number of log lines to show")
	cmd.Flags().TimeVar(&since, "since", time.Time{}, []string{time.RFC3339}, "Fetch logs after this timestamp (RFC3339 format, e.g., 2024-01-15T09:00:00Z)")
	cmd.Flags().TimeVar(&until, "until", time.Time{}, []string{time.RFC3339}, "Fetch logs before this timestamp (RFC3339 format, e.g., 2024-01-15T10:00:00Z)")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (text, json, yaml)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}
