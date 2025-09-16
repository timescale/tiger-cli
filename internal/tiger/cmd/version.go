package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// These variables are set at build time via ldflags in the GoReleaser pipeline
// for production releases. Default values are used for local development builds.
var Version = "dev"
var BuildTime = "unknown"
var GitCommit = "unknown"

func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Display version, build time, and git commit information for the Tiger CLI`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Tiger CLI %s\n", Version)
			fmt.Printf("Build time: %s\n", BuildTime)
			fmt.Printf("Git commit: %s\n", GitCommit)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}
