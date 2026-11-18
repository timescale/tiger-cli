// Package testutil holds the setup shared by every package's tests. Only
// _test.go files may import it, so it never reaches the tiger binary.
package testutil

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// Main runs a package's tests from the baseline every package shares. Each
// package's TestMain calls it, so the baseline can't drift between packages.
func Main(m *testing.M) {
	// Backstop: replace the system keyring with an in-memory mock, so even a
	// test that forgets to reset it can never read, write, or delete real
	// credentials or passwords. Per-test isolation comes from a fresh
	// keyring.MockInit() in each package's harness and in tests that use the
	// keyring directly.
	keyring.MockInit()

	if err := scrubEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to scrub the environment: %v\n", err)
		os.Exit(1)
	}

	// Pin the local timezone to UTC so output that renders local times is
	// deterministic. This must happen here, while the process is still
	// single-goroutine: mutating time.Local mid-run races with any background
	// goroutine that calls time.Now, such as an idle HTTP transport's read
	// loop left over from an earlier test.
	time.Local = time.UTC

	os.Exit(m.Run())
}

// scrubEnv removes inherited env vars (from the developer's shell or a sourced
// .env file) that would change what tests resolve, so tests run from the same
// baseline everywhere. Tests that need one set it with t.Setenv.
//
//   - TIGER_* vars feed config.Load through viper's TIGER prefix. Integration
//     test credentials (TIGER_*_INTEGRATION) are kept.
//   - PG* vars are read by pgx and libpq (PGPASSWORD, PGCONNECT_TIMEOUT,
//     PGHOST, ...) and inherited by psql, so they would change the connection
//     configs and psql environments tests check.
func scrubEnv() error {
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		tiger := strings.HasPrefix(key, "TIGER_") && !strings.HasSuffix(key, "_INTEGRATION")
		if tiger || strings.HasPrefix(key, "PG") {
			if err := os.Unsetenv(key); err != nil {
				return fmt.Errorf("unset %s: %w", key, err)
			}
		}
	}
	return nil
}
