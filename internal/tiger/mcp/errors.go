package mcp

import (
	"errors"
	"strings"

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

// buildServerInstructions returns the `instructions` string the MCP SDK
// sends to clients at initialize. Empty when read-only is off so the SDK
// omits the field.
//
// Instructions are evaluated once at server start; toggling read_only
// mid-session leaves the warning stale until the MCP client restarts. The
// gate itself stays correct because handlers reload config per call.
func buildServerInstructions(cfg *config.Config) string {
	if cfg == nil || !cfg.ReadOnly {
		return ""
	}
	return "READ-ONLY MODE IS ENABLED. The following Tiger MCP tools will refuse to run: " +
		strings.Join(readOnlyGatedTools, ", ") + ". " +
		"Before asking the user to provide inputs for any of these operations, " +
		"tell them read-only mode is on and ask whether to disable it " +
		"(run: `tiger config unset read_only`) or skip the operation."
}
