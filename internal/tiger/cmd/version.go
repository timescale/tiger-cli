package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
	"github.com/timescale/tiger-cli/internal/tiger/version"
	"gopkg.in/yaml.v3"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			versionOutput := VersionOutput{
				Version:   config.Version,
				BuildTime: config.BuildTime,
				GitCommit: config.GitCommit,
				GoVersion: runtime.Version(),
				Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			}

			if checkVersion {
				cfg, err := config.Load()
				if err != nil {
					return fmt.Errorf("Error loading config: %w", err)
				}
				if result := version.PerformCheck(cfg, util.Ptr(cmd.ErrOrStderr()), true); result != nil {
					versionOutput.LatestVersion = result.LatestVersion
					versionOutput.UpdateAvailable = &result.UpdateAvailable
				}
			}

			output := cmd.OutOrStdout()
			switch outputFormat {
			case "json":
				encoder := json.NewEncoder(output)
				encoder.SetIndent("", "  ")
				return encoder.Encode(versionOutput)
			case "yaml":
				encoder := yaml.NewEncoder(output)
				encoder.SetIndent(2)
				jsonArray, err := toJSON(versionOutput)
				if err != nil {
					return fmt.Errorf("failed to convert version info to map: %w", err)
				}
				return encoder.Encode(jsonArray)
			case "bare":
				fmt.Fprintln(output, versionOutput.Version)
				return nil
			default:
				return outputVersionTable(versionOutput, output)
			}
		},
	}

	cmd.Flags().BoolVar(&checkVersion, "check", false, "Check for the latest version")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml, bare)")

	return cmd
}

func outputVersionTable(versionOutput VersionOutput, output io.Writer) error {
	table := tablewriter.NewWriter(output)

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
