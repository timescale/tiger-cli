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
		want       int
	}{
		{
			name:       "configured value is used",
			configured: 250,
			want:       250,
		},
		{
			// A config-file or TIGER_MCP_MAX_ROWS value bypasses `tiger config
			// set` validation, so a zero (or negative) configured value can
			// reach here and must be sanitized to the default.
			name:       "zero configured (env/file bypass) falls back to default",
			configured: 0,
			want:       config.DefaultMCPMaxRows,
		},
		{
			name:       "negative configured falls back to default",
			configured: -1,
			want:       config.DefaultMCPMaxRows,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveMaxRows(tt.configured); got != tt.want {
				t.Errorf("resolveMaxRows(%d) = %d, want %d", tt.configured, got, tt.want)
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
