package cmd

import (
	"maps"
	"regexp"
	"strings"
	"testing"
)

// noDocsProxy returns run options that disable the remote docs MCP proxy
// (plus any extra config values), so tests that build an MCP server never
// reach the network. Only natively registered tools appear in listings; the
// proxied docs tools, prompts, and resources are absent.
func noDocsProxy(extra map[string]any) []runOption {
	values := map[string]any{"docs_mcp": false}
	maps.Copy(values, extra)
	return []runOption{withConfig(values)}
}

// logTimestamp matches the "2026/08/27 17:49:25 " prefix that the standard log
// package stamps on every line the MCP server's slog handler writes (see
// newLogger). Only `tiger mcp start` logs at all, so the stripping helpers
// below live with the other MCP test scaffolding.
var logTimestamp = regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)

// matchLog asserts the stream equals want once the per-line timestamps are
// stripped, so log output can still be asserted exactly.
func matchLog(want string) matcher {
	return matchFunc(func(t *testing.T, got string) {
		t.Helper()
		assertOutput(t, logTimestamp.ReplaceAllString(got, ""), want)
	})
}

// matchLogPort is matchLog for output naming a port the OS picked rather than
// one the test chose. The want text is matched literally except for "<port>",
// which stands in for any port number.
func matchLogPort(want string) matcher {
	re := regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(want), "<port>", `\d+`) + "$")
	return matchFunc(func(t *testing.T, got string) {
		t.Helper()
		if stripped := logTimestamp.ReplaceAllString(got, ""); !re.MatchString(stripped) {
			t.Errorf("log = %q, want %q (<port> matching any port)", stripped, want)
		}
	})
}
