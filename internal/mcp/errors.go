package mcp

import (
	"errors"
	"fmt"

	"github.com/timescale/tiger-cli/internal/common"
)

// handleDatabaseError turns the readiness sentinels into guidance naming the
// tool that resolves them: a branch tool for a branch, a service tool for
// everything else. Every other error passes through unchanged.
func handleDatabaseError(err error, target *common.ConnectionTarget) error {
	// The group also names the target, since the sentinels always say
	// "service".
	group := "service"
	if target.Kind == common.TargetBranch {
		group = "branch"
	}

	switch {
	case errors.Is(err, common.ErrPaused):
		return fmt.Errorf("%s is paused — start it with the %s_start tool", group, group)
	case errors.Is(err, common.ErrNotReady):
		return fmt.Errorf("%s is not ready — check its status with %s_get and try again", group, group)
	}
	return err
}
