package util

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal is a helper for detecting whether an [io.Writer] or [io.Reader]
// is an interactive terminal / TTY. It is a variable so that tests can
// override it.
var IsTerminal = func(v any) bool {
	if f, ok := v.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// IsCI determines if the current execution context is within a known CI/CD system.
// This is based on https://github.com/watson/ci-info/blob/HEAD/index.js.
func IsCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}
