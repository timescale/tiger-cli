package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Display version, build time, and git commit information for the Tiger CLI`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Tiger CLI %s\n", config.Version)
			fmt.Printf("Build time: %s\n", config.BuildTime)
			fmt.Printf("Git commit: %s\n", config.GitCommit)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}
