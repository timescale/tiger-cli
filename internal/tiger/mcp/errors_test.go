package mcp

import (
	"errors"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestCheckReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		readOnly bool
		wantErr  error
	}{
		{name: "read-only off allows writes", readOnly: false, wantErr: nil},
		{name: "read-only on blocks writes", readOnly: true, wantErr: errReadOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ReadOnly: tt.readOnly}
			err := checkReadOnly(cfg)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("checkReadOnly(ReadOnly=%t) error = %v, want %v", tt.readOnly, err, tt.wantErr)
			}
		})
	}
}

func TestBuildServerInstructions(t *testing.T) {
	if got := buildServerInstructions(nil); got != "" {
		t.Errorf("nil cfg should produce empty instructions, got %q", got)
	}

	if got := buildServerInstructions(&config.Config{ReadOnly: false}); got != "" {
		t.Errorf("read-only off should produce empty instructions, got %q", got)
	}

	got := buildServerInstructions(&config.Config{ReadOnly: true})
	if got == "" {
		t.Fatal("read-only on should produce non-empty instructions")
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
