package config

import (
	"fmt"
	"runtime"
)

// These variables are set at build time via ldflags in the GoReleaser pipeline
// for production releases. Default values are used for local development builds.
var Version = "dev"
var BuildTime = "unknown"
var GitCommit = "unknown"

// UserAgent returns the User-Agent the CLI sends on HTTP requests.
func UserAgent() string {
	return fmt.Sprintf("tiger-cli/%s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
}
