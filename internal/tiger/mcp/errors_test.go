package mcp

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestBuildServerInstructions(t *testing.T) {
	const capabilitiesMarker = "Tiger MCP provides tools"

	readWrite := buildServerInstructions(&config.Config{ReadOnly: false})
	readOnly := buildServerInstructions(&config.Config{ReadOnly: true})

	// The capabilities blurb is always present, including for a nil config.
	for _, got := range []string{buildServerInstructions(nil), readWrite, readOnly} {
		if !strings.Contains(got, capabilitiesMarker) {
			t.Errorf("instructions missing capabilities blurb: %q", got)
		}
	}

	// The read-only banner appears only in read-only mode.
	const banner = "READ-ONLY MODE IS ENABLED"
	if !strings.Contains(readOnly, banner) {
		t.Errorf("read-only instructions missing banner: %q", readOnly)
	}
	if strings.Contains(readWrite, banner) {
		t.Errorf("read-write instructions should not contain banner: %q", readWrite)
	}

	// Read-only instructions never name the gated write tools.
	for _, tool := range readOnlyGatedTools {
		if strings.Contains(readOnly, tool) {
			t.Errorf("read-only instructions should not name gated tool %q: %q", tool, readOnly)
		}
	}
}
