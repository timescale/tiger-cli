package mcp

import (
	"errors"
	"fmt"

	"github.com/timescale/tiger-cli/internal/common"
)

// readOnlyGatedTools are the service-mutating tools addTool skips under
// read_only=all.
var readOnlyGatedTools = []string{
	toolServiceCreate,
	toolServiceFork,
	toolServiceStart,
	toolServiceStop,
	toolServiceResize,
	toolServiceUpdatePassword,
}

// handleDatabaseError turns the readiness sentinels into guidance naming the
// tool that resolves them. Every other error passes through unchanged.
func handleDatabaseError(err error) error {
	switch {
	case errors.Is(err, common.ErrPaused):
		return fmt.Errorf("%w — start it with the service_start tool", common.ErrPaused)
	case errors.Is(err, common.ErrNotReady):
		return fmt.Errorf("%w — check its status with service_get and try again", common.ErrNotReady)
	}
	return err
}
