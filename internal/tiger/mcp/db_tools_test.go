package mcp

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestResolveMaxRows(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		requested  int
		want       int
	}{
		{
			name:       "both unset falls back to default",
			configured: 0,
			requested:  0,
			want:       config.DefaultMCPMaxRows,
		},
		{
			name:       "configured used when no per-call value",
			configured: 250,
			requested:  0,
			want:       250,
		},
		{
			name:       "per-call overrides configured",
			configured: 250,
			requested:  10,
			want:       10,
		},
		{
			name:       "per-call overrides default when configured unset",
			configured: 0,
			requested:  42,
			want:       42,
		},
		{
			name:       "per-call clamped to ceiling",
			configured: 100,
			requested:  mcpMaxRowsCeiling + 5000,
			want:       mcpMaxRowsCeiling,
		},
		{
			name:       "configured clamped to ceiling",
			configured: mcpMaxRowsCeiling + 1,
			requested:  0,
			want:       mcpMaxRowsCeiling,
		},
		{
			// A config-file or TIGER_MCP_MAX_ROWS value bypasses `tiger config
			// set` validation, so a zero (or negative) configured value can
			// reach here and must be sanitized to the default.
			name:       "zero configured (env/file bypass) falls back to default",
			configured: 0,
			requested:  0,
			want:       config.DefaultMCPMaxRows,
		},
		{
			name:       "negative configured falls back to default",
			configured: -1,
			requested:  0,
			want:       config.DefaultMCPMaxRows,
		},
		{
			name:       "negative per-call value is ignored in favor of configured",
			configured: 250,
			requested:  -5,
			want:       250,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveMaxRows(tt.configured, tt.requested); got != tt.want {
				t.Errorf("resolveMaxRows(%d, %d) = %d, want %d", tt.configured, tt.requested, got, tt.want)
			}
		})
	}
}

func TestApproxRowSize(t *testing.T) {
	// A small row should be smaller than a row with a large text value, and
	// both should be positive. We don't assert exact byte counts (they track
	// JSON encoding), only the ordering and positivity the byte budget relies on.
	small := approxRowSize([]any{1, "a"})
	large := approxRowSize([]any{1, strings.Repeat("x", 1000)})

	if small <= 0 {
		t.Errorf("approxRowSize(small) = %d, want > 0", small)
	}
	if large <= small {
		t.Errorf("approxRowSize(large)=%d should exceed approxRowSize(small)=%d", large, small)
	}
}

func TestTruncationNotice(t *testing.T) {
	notice := truncationNotice(100)
	// The notice must mention the actual cap and steer the model toward doing
	// the work in SQL rather than re-running the query.
	for _, want := range []string{"100", "LIMIT", "aggregate"} {
		if !strings.Contains(notice, want) {
			t.Errorf("truncationNotice() = %q, missing %q", notice, want)
		}
	}
}

func TestDBExecuteQueryInputSchemaHasMaxRows(t *testing.T) {
	schema := DBExecuteQueryInput{}.Schema()
	prop, ok := schema.Properties["max_rows"]
	if !ok {
		t.Fatal("expected max_rows property in input schema")
	}
	if prop.Description == "" {
		t.Error("expected max_rows to have a description")
	}
	// No default: omitting max_rows must fall back to the configured value,
	// not a schema-injected default.
	if prop.Default != nil {
		t.Errorf("expected max_rows to have no schema default, got %s", string(prop.Default))
	}
}

func TestDBExecuteQueryOutputSchemaHasTruncationFields(t *testing.T) {
	schema := DBExecuteQueryOutput{}.Schema()
	for _, name := range []string{"truncated", "notice"} {
		prop, ok := schema.Properties[name]
		if !ok {
			t.Fatalf("expected %q property in output schema", name)
		}
		if prop.Description == "" {
			t.Errorf("expected %q to have a description", name)
		}
	}
	resultSet := schema.Properties["result_sets"].Items
	if _, ok := resultSet.Properties["truncated"]; !ok {
		t.Error("expected truncated property on result set schema")
	}
}
