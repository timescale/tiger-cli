package common

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// Backstop: replace the system keyring with an in-memory mock so that even
	// a test that forgets to reset can never read, write, or delete real
	// credentials or passwords. Per-test isolation comes from the fresh
	// keyring.MockInit() in each test that uses the keyring.
	keyring.MockInit()

	// Scrub inherited TIGER_* env vars (e.g. from the developer's shell or a
	// sourced .env file): config.Load reads them through viper's TIGER prefix,
	// so a stray TIGER_API_URL or TIGER_OUTPUT changes what these tests
	// resolve. Integration test credentials (TIGER_*_INTEGRATION) are
	// deliberately preserved.
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "TIGER_") && !strings.HasSuffix(key, "_INTEGRATION") {
			os.Unsetenv(key)
		}
	}

	// Pin the local timezone to UTC so any output rendering local times is
	// deterministic. This must happen here, while the process is still
	// single-goroutine: mutating time.Local mid-run races with any background
	// goroutine that calls time.Now.
	time.Local = time.UTC

	os.Exit(m.Run())
}
