package restore

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestFormatRowCount(t *testing.T) {
	tests := []struct {
		name     string
		count    int64
		expected string
	}{
		{"less than 1000", 999, "999"},
		{"exactly 1000", 1000, "1.0K"},
		{"thousands", 5500, "5.5K"},
		{"exactly 1 million", 1000000, "1.0M"},
		{"millions", 2500000, "2.5M"},
		{"exactly 1 billion", 1000000000, "1.0B"},
		{"billions", 3500000000, "3.5B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRowCount(tt.count)
			if result != tt.expected {
				t.Errorf("formatRowCount(%d) = %s; want %s", tt.count, result, tt.expected)
			}
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected string
	}{
		{"zero", 0, "s"},
		{"one", 1, ""},
		{"two", 2, "s"},
		{"many", 100, "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pluralize(tt.count)
			if result != tt.expected {
				t.Errorf("pluralize(%d) = %s; want %s", tt.count, result, tt.expected)
			}
		})
	}
}

func TestRestorePlainSQLWithPsql_OutputModes(t *testing.T) {
	// Save original getNow and restore after test
	originalGetNow := getNow
	defer func() { getNow = originalGetNow }()

	// Mock time for consistent duration
	mockTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	getNow = func() time.Time { return mockTime }

	tests := []struct {
		name    string
		quiet   bool
		verbose bool
		wantOut string
	}{
		{
			name:    "default mode",
			quiet:   false,
			verbose: false,
			wantOut: "", // Should suppress all psql output
		},
		{
			name:    "verbose mode",
			quiet:   false,
			verbose: true,
			wantOut: "", // Would show psql output if psql was actually run
		},
		{
			name:    "quiet mode",
			quiet:   true,
			verbose: false,
			wantOut: "", // Should suppress everything
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test restorer
			cfg := &config.Config{}
			opts := &Options{
				ServiceID: "test-service",
				FilePath:  "test.sql",
				Quiet:     tt.quiet,
				Verbose:   tt.verbose,
				Role:      "tsdbadmin",
			}

			var outBuf, errBuf bytes.Buffer
			opts.Output = &outBuf
			opts.Errors = &errBuf

			_ = NewRestorer(cfg, opts)

			// Note: This test doesn't actually run psql, just tests the setup logic
			// Full integration tests would require a real database
		})
	}
}

func TestRestoreSummaryFormatting(t *testing.T) {
	tests := []struct {
		name     string
		summary  *RestoreSummary
		duration time.Duration
		wantText []string // Strings that should appear in output
	}{
		{
			name: "single table",
			summary: &RestoreSummary{
				Tables:    1,
				TotalRows: 1000,
			},
			duration: 1500 * time.Millisecond,
			wantText: []string{"1 table", "1.0K rows"},
		},
		{
			name: "multiple tables and indexes",
			summary: &RestoreSummary{
				Tables:    5,
				Indexes:   10,
				TotalRows: 1500000,
			},
			duration: 3 * time.Second,
			wantText: []string{"5 tables", "10 indexes", "1.5M rows"},
		},
		{
			name: "hypertables",
			summary: &RestoreSummary{
				Tables:       3,
				Hypertables:  2,
				HasTimescale: true,
				TotalRows:    50000,
			},
			duration: 2 * time.Second,
			wantText: []string{"3 tables", "2 hypertables", "50.0K rows"},
		},
		{
			name: "all object types",
			summary: &RestoreSummary{
				Tables:       10,
				Views:        3,
				Functions:    5,
				Sequences:    2,
				Indexes:      15,
				Hypertables:  4,
				HasTimescale: true,
				TotalRows:    2000000000,
			},
			duration: 30 * time.Second,
			wantText: []string{
				"10 tables",
				"3 views",
				"5 functions",
				"2 sequences",
				"15 indexes",
				"4 hypertables",
				"2.0B rows",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We test the formatting logic directly without creating a restorer
			// since we can't mock database connections in unit tests

			// Test row count formatting
			if tt.summary.TotalRows > 0 {
				formatted := formatRowCount(tt.summary.TotalRows)
				found := false
				for _, want := range tt.wantText {
					if want == formatted+" rows" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("formatRowCount(%d) = %s; expected to match one of %v",
						tt.summary.TotalRows, formatted, tt.wantText)
				}
			}

			// Test pluralization
			if tt.summary.Tables > 0 {
				_ = pluralize(tt.summary.Tables)
				found := false
				for _, want := range tt.wantText {
					if want == formatStatCount(tt.summary.Tables, "table") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected pluralization for %d tables", tt.summary.Tables)
				}
			}
		})
	}
}

// Helper function to format stat counts consistently
func formatStatCount(count int, singular string) string {
	return fmt.Sprintf("%d %s%s", count, singular, pluralize(count))
}

// Helper to create string pointers
func stringPtr(s string) *string {
	return &s
}
