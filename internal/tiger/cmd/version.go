package cmd

import (
	"fmt"
	"io"
	"runtime"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
	"github.com/timescale/tiger-cli/internal/tiger/version"
)

type VersionOutput struct {
	Version         string `json:"version" yaml:"version"`
	BuildTime       string `json:"build_time" yaml:"build_time"`
	GitCommit       string `json:"git_commit" yaml:"git_commit"`
	GoVersion       string `json:"go_version" yaml:"go_version"`
	Platform        string `json:"platform" yaml:"platform"`
	LatestVersion   string `json:"latest_version,omitempty" yaml:"latest_version,omitempty"`
	UpdateAvailable *bool  `json:"update_available,omitempty" yaml:"update_available,omitempty"`
}

func buildVersionCmd() *cobra.Command {
	var checkVersion bool
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Display version, build time, and git commit information for the Tiger CLI`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) (runErr error) {
			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			versionOutput := VersionOutput{
				Version:   config.Version,
				BuildTime: config.BuildTime,
				GitCommit: config.GitCommit,
				GoVersion: runtime.Version(),
				Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			}

			updateAvailable := false
			if checkVersion {
				if result := version.PerformCheck(cfg, util.Ptr(cmd.ErrOrStderr()), true); result != nil {
					versionOutput.LatestVersion = result.LatestVersion
					versionOutput.UpdateAvailable = &result.UpdateAvailable
					updateAvailable = result.UpdateAvailable
					// Print warning _after_ other output
					defer version.PrintUpdateWarning(result, cfg, util.Ptr(cmd.ErrOrStderr()))
				}
			}

			// Track analytics
			a := analytics.TryInit(cfg)
			defer func() {
				a.Track("Run tiger version",
					analytics.FlagSet(cmd.LocalFlags()),
					analytics.NonZero("latest_version", versionOutput.LatestVersion),
					analytics.Error(runErr),
				)
			}()

			output := cmd.OutOrStdout()
			switch outputFormat {
			case "json":
				if err := util.SerializeToJSON(output, versionOutput); err != nil {
					return err
				}
			case "yaml":
				if err := util.SerializeToYAML(output, versionOutput, true); err != nil {
					return err
				}
			case "bare":
				fmt.Fprintln(output, versionOutput.Version)
			default:
				if err := outputVersionTable(output, versionOutput); err != nil {
					return err
				}
			}
			if updateAvailable {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				return exitWithCode(ExitUpdateAvailable, nil)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkVersion, "check", false, "Force checking for updates (regardless of last check time)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml, bare)")

	return cmd
}

func outputVersionTable(w io.Writer, versionOutput VersionOutput) error {
	table := tablewriter.NewWriter(w)

	table.Append("Tiger CLI Version", versionOutput.Version)
	if versionOutput.LatestVersion != "" {
		table.Append("Latest Version", versionOutput.LatestVersion)
		table.Append("Update Available", fmt.Sprintf("%v", util.Deref(versionOutput.UpdateAvailable)))
	}
	table.Append("Build Time", versionOutput.BuildTime)
	table.Append("Git Commit", versionOutput.GitCommit)
	table.Append("Go Version", versionOutput.GoVersion)
	table.Append("Platform", versionOutput.Platform)

	return table.Render()
}
