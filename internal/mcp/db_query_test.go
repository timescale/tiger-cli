package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
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

func TestDBQueryOutputSchemaHasTruncationFields(t *testing.T) {
	schema := DBQueryOutput{}.Schema()
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

func TestResolveQueryInput(t *testing.T) {
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "schema.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatalf("failed to write SQL file: %v", err)
	}
	emptyPath := filepath.Join(dir, "empty.sql")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("failed to write empty SQL file: %v", err)
	}

	tests := []struct {
		name    string
		query   string
		file    string
		want    string
		wantErr string
	}{
		{
			name:  "inline query",
			query: "SELECT 1",
			want:  "SELECT 1",
		},
		{
			name: "query read from file",
			file: sqlPath,
			want: "SELECT 1;\n",
		},
		{
			name:    "neither provided",
			wantErr: "exactly one of 'query' or 'file' must be provided",
		},
		{
			name:    "both provided",
			query:   "SELECT 1",
			file:    sqlPath,
			wantErr: "exactly one of 'query' or 'file' must be provided",
		},
		{
			name:    "missing file",
			file:    filepath.Join(dir, "nope.sql"),
			wantErr: "failed to read SQL file",
		},
		{
			name:    "empty file",
			file:    emptyPath,
			wantErr: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveQueryInput(tt.query, tt.file)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("query = %q, want %q", got, tt.want)
			}
		})
	}
}
