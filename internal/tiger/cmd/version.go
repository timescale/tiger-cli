package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

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

func init() {
	versionCmd := buildVersionCmd()
	rootCmd.AddCommand(versionCmd)
}