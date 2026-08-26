package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// discardCmd returns a bare command whose output streams are discarded, for
// tests that call a helper taking a *cobra.Command without caring what it
// prints.
func discardCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// stubIsTerminal makes util.IsTerminal report val for the duration of the test,
// so commands take their interactive path against a non-TTY stdin.
func stubIsTerminal(t *testing.T, val bool) {
	t.Helper()
	original := util.IsTerminal
	util.IsTerminal = func(any) bool { return val }
	t.Cleanup(func() { util.IsTerminal = original })
}

// stubReadPassword makes util.ReadPassword return password for the duration of
// the test. The real implementation needs stdin to be an *os.File, so password
// prompts can't be driven with cmd.SetIn.
func stubReadPassword(t *testing.T, password string) {
	t.Helper()
	original := util.ReadPassword
	util.ReadPassword = func(context.Context, io.Reader) (string, error) { return password, nil }
	t.Cleanup(func() { util.ReadPassword = original })
}

func TestMain(m *testing.M) {
	// Clean up any global state before tests
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
	}

	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

// testConfigDir returns the config directory the test is using: the one its setup
// helper exported via TIGER_CONFIG_DIR, or an isolated empty directory when the
// test set none — so a config never leaks in from the machine running the tests.
// Tests that exercise credential storage need the former, since credentials live
// in this directory (the auth setup helpers set it).
func testConfigDir(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("TIGER_CONFIG_DIR"); dir != "" {
		return dir
	}
	return t.TempDir()
}

// testFlags returns a flag set shaped like a command's, with --config-dir pointed
// at dir, so config.Load resolves the same way it would for a real command.
func testFlags(t *testing.T, dir string) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("config-dir", "", "config directory")
	if err := flags.Set("config-dir", dir); err != nil {
		t.Fatalf("Failed to set config-dir flag: %v", err)
	}
	return flags
}

// testConfig loads the config for the test's config directory. Use it where a
// test needs the config itself (credential storage, password storage) rather than
// running a command.
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(testFlags(t, testConfigDir(t)))
	if err != nil {
		t.Fatalf("Failed to load test config: %v", err)
	}
	return cfg
}

// newTestApp returns an App loaded against the test's config directory, with API
// client creation stubbed out to return the given client. Load still runs, so
// config resolution and flag precedence go through the real code path — only the
// client is injected (see common.App.SetClientFactory).
func newTestApp(t *testing.T, client api.ClientWithResponsesInterface, projectID string) *common.App {
	t.Helper()
	app := &common.App{}
	app.SetFlags(testFlags(t, testConfigDir(t)))
	app.SetClientFactory(func(context.Context, *config.Config) (api.ClientWithResponsesInterface, string, error) {
		return client, projectID, nil
	})
	if _, _, _, err := app.Load(t.Context()); err != nil {
		t.Fatalf("Failed to load test app: %v", err)
	}
	return app
}

// assertExitCode checks that err carries the given CLI exit code, unwrapping
// like cmd/tiger/main.go does.
func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	var codeErr common.ExitCodeError
	if !errors.As(err, &codeErr) || codeErr.ExitCode() != want {
		t.Errorf("Expected exit code %d, got: %v", want, err)
	}
}

// mockStoredCredentials overrides the common.GetStoredCredentials seam for the
// duration of the test, restoring the original automatically via t.Cleanup.
func mockStoredCredentials(t *testing.T, creds *config.Credentials, err error) {
	t.Helper()
	original := common.GetStoredCredentials
	common.GetStoredCredentials = func(*config.Config) (*config.Credentials, error) {
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
