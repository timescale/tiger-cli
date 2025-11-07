package restore

import (
	"fmt"
	"time"
)

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// formatDuration formats a duration into human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// getNow returns the current time (can be mocked in tests)
var getNow = time.Now

// RestoreError represents an error during restore
type RestoreError struct {
	Phase         string // "preflight", "pre_restore", "restore", "post_restore"
	Statement     string
	LineNumber    int
	PostgresError string
	Recoverable   bool
}

// Error implements the error interface
func (e *RestoreError) Error() string {
	if e.LineNumber > 0 {
		return fmt.Sprintf("%s failed at line %d: %s", e.Phase, e.LineNumber, e.PostgresError)
	}
	return fmt.Sprintf("%s failed: %s", e.Phase, e.PostgresError)
}
