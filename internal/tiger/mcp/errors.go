package mcp

import (
	"errors"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

var errReadOnly = errors.New("this operation is not allowed in read-only mode (unset with: tiger config unset read_only)")

// readOnlyGatedTools shares the gated-tool list between the gate and the
// server-instructions warning so they can't drift.
var readOnlyGatedTools = []string{
	toolServiceCreate,
	toolServiceFork,
	toolServiceStart,
	toolServiceStop,
	toolServiceResize,
	toolServiceUpdatePassword,
}

func checkReadOnly(cfg *config.Config) error {
	if cfg.ReadOnly {
		return errReadOnly
	}
	return nil
}
