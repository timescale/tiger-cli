package cmd

import (
	"fmt"
	"io"
	"runtime"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
	"github.com/timescale/tiger-cli/internal/version"
)

type VersionOutput struct {
	Version         string `json:"version"`
	BuildTime       string `json:"build_time"`
	GitCommit       string `json:"git_commit"`
	GoVersion       string `json:"go_version"`
	Platform        string `json:"platform"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable *bool  `json:"update_available,omitempty"`
}

func buildVersionCmd(app *common.App) *cobra.Command {
	var checkVersion bool

	cmd := &cobra.Command{
		Use:               "version",
		Short:             "Show version information",
		Long:              `Display version, build time, and git commit information for the Tiger CLI`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			versionOutput := VersionOutput{
				Version:   config.Version,
				BuildTime: config.BuildTime,
				GitCommit: config.GitCommit,
				GoVersion: runtime.Version(),
				Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			}

			updateAvailable := false
			if checkVersion {
				result, err := version.CheckForUpdate(cmd.Context(), cfg)
				if err != nil {
					// A failed check shouldn't fail the version command; warn and
					// continue printing the local version info.
					cmd.PrintErrf("Warning: failed to check for updates: %v\n", err)
				} else if result != nil {
					versionOutput.LatestVersion = result.LatestVersion
					versionOutput.UpdateAvailable = &result.UpdateAvailable
					updateAvailable = result.UpdateAvailable
					// Print warning _after_ other output
					defer version.PrintUpdateWarning(result, cfg, cmd.ErrOrStderr())
				}
			}

			output := cmd.OutOrStdout()
			switch cfg.Output {
			case "json":
				if err := util.SerializeToJSON(output, versionOutput); err != nil {
					return err
				}
			case "yaml":
				if err := util.SerializeToYAML(output, versionOutput); err != nil {
					return err
				}
			case "bare":
				cmd.Println(versionOutput.Version)
			default:
				if err := outputVersionTable(output, versionOutput); err != nil {
					return err
				}
			}
			if updateAvailable {
				cmd.SilenceErrors = true
				return common.ExitWithCode(common.ExitUpdateAvailable, nil)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkVersion, "check", false, "Force checking for updates (regardless of last check time)")
	cmd.Flags().VarP(new(outputWithBareFlag), "output", "o", "Output format (table, json, yaml, bare)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion("bare"))

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
