package cmd

import (
	"os"
	"testing"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestMain(m *testing.M) {
	// Clean up any global state before tests
	config.ResetGlobalConfig()
	code := m.Run()
	os.Exit(code)
}

func setupTestCommand(t *testing.T) (string, func()) {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-test-cmd-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Disable analytics for root tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	// Clean up function
	cleanup := func() {
		os.RemoveAll(tmpDir)
		os.Unsetenv("TIGER_ANALYTICS")
		config.ResetGlobalConfig()
	}

	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

// mockStoredCredentials overrides the common.GetStoredCredentials seam for the
// duration of the test, restoring the original automatically via t.Cleanup.
func mockStoredCredentials(t *testing.T, creds *config.Credentials, err error) {
	t.Helper()
	original := common.GetStoredCredentials
	common.GetStoredCredentials = func() (*config.Credentials, error) {
		return creds, err
	}
	t.Cleanup(func() { common.GetStoredCredentials = original })
}

// mockTestPAT injects a fixed PAT credential.
func mockTestPAT(t *testing.T) {
	mockStoredCredentials(t, &config.Credentials{
		APIKey:    "test-api-key",
		ProjectID: "test-project-123",
	}, nil)
}

// mockNotLoggedIn simulates the absence of stored credentials.
func mockNotLoggedIn(t *testing.T) {
	mockStoredCredentials(t, nil, config.ErrNotLoggedIn)
}
