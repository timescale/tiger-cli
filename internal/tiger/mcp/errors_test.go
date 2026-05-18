package mcp

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestBuildServerInstructions(t *testing.T) {
	const capabilitiesMarker = "Tiger MCP provides tools"

	for _, tt := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil cfg", cfg: nil},
		{name: "read-only off", cfg: &config.Config{ReadOnly: false}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := buildServerInstructions(tt.cfg)
			if !strings.Contains(got, capabilitiesMarker) {
				t.Errorf("instructions missing capabilities blurb: %q", got)
			}
			if strings.Contains(got, "READ-ONLY MODE IS ENABLED") {
				t.Errorf("instructions should not contain read-only banner when read-only is off: %q", got)
			}
		})
	}

	got := buildServerInstructions(&config.Config{ReadOnly: true})
	if !strings.Contains(got, capabilitiesMarker) {
		t.Errorf("instructions missing capabilities blurb: %q", got)
	}
	if !strings.Contains(got, "READ-ONLY MODE IS ENABLED") {
		t.Errorf("instructions missing read-only banner: %q", got)
	}
	for _, tool := range readOnlyGatedTools {
		if !strings.Contains(got, tool) {
			t.Errorf("instructions missing gated tool %q in: %q", tool, got)
		}
	}
}
