package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// withCLIVersion overrides the build-time version string for a test case, so
// paths gated on a released (semver) version are reachable from a dev build.
// Restored by the running subtest's cleanups.
func withCLIVersion(v string) runOption {
	return withSetup(func(t *testing.T) {
		original := config.Version
		config.Version = v
		t.Cleanup(func() { config.Version = original })
	})
}

func TestVersionCmd(t *testing.T) {
	goVersion := runtime.Version()
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	baseInfo := VersionOutput{
		Version:   config.Version,
		BuildTime: config.BuildTime,
		GitCommit: config.GitCommit,
		GoVersion: goVersion,
		Platform:  platform,
	}
	// The table renderer sizes columns to fit go_version and platform, which
	// vary by toolchain and OS, so the expected table output is rendered with
	// the same helper the command uses rather than pasted as a literal.
	renderTable := func(info VersionOutput) string {
		var buf bytes.Buffer
		if err := outputVersionTable(&buf, info); err != nil {
			t.Fatalf("outputVersionTable: %v", err)
		}
		return buf.String()
	}

	// config.Version is "dev" in tests, so a successful --check never reports
	// an update as available (dev builds never compare as older).
	upToDateServer := startMockReleasesServer(t, "v99.99.99")
	upToDateInfo := baseInfo
	upToDateInfo.LatestVersion = "99.99.99"
	upToDateInfo.UpdateAvailable = new(false)

	brokenServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(brokenServer.Close)

	tests := []cmdTest{
		{
			name:    "rejects positional args",
			args:    []string{"version", "extra"},
			wantErr: `unknown command "extra" for "tiger version"`,
		},
		{
			name:    "rejects invalid output format",
			args:    []string{"version", "-o", "bogus"},
			wantErr: `invalid argument "bogus" for "-o, --output" flag: invalid output format: bogus (must be one of: json, yaml, table, bare)`,
		},
		{
			name:       "table output",
			args:       []string{"version"},
			wantStdout: renderTable(baseInfo),
		},
		{
			name: "json output",
			args: []string{"version", "-o", "json"},
			wantStdout: fmt.Sprintf(`{
  "version": %q,
  "build_time": %q,
  "git_commit": %q,
  "go_version": %q,
  "platform": %q
}
`, config.Version, config.BuildTime, config.GitCommit, goVersion, platform),
		},
		{
			name: "yaml output",
			args: []string{"version", "-o", "yaml"},
			wantStdout: fmt.Sprintf(`build_time: %s
git_commit: %s
go_version: %s
platform: %s
version: %s
`, config.BuildTime, config.GitCommit, goVersion, platform, config.Version),
		},
		{
			name:       "bare output",
			args:       []string{"version", "-o", "bare"},
			wantStdout: config.Version + "\n",
		},
		{
			name:       "check with no update available",
			args:       []string{"version", "--check"},
			opts:       []runOption{withConfig(map[string]any{"releases_url": upToDateServer.URL})},
			wantStdout: renderTable(upToDateInfo),
		},
		{
			name: "check with no update available json output",
			args: []string{"version", "--check", "-o", "json"},
			opts: []runOption{withConfig(map[string]any{"releases_url": upToDateServer.URL})},
			wantStdout: fmt.Sprintf(`{
  "version": %q,
  "build_time": %q,
  "git_commit": %q,
  "go_version": %q,
  "platform": %q,
  "latest_version": "99.99.99",
  "update_available": false
}
`, config.Version, config.BuildTime, config.GitCommit, goVersion, platform),
		},
		{
			// A failed check warns but still prints the local version info and
			// exits successfully.
			name:       "check failure warns and still prints version",
			args:       []string{"version", "--check"},
			opts:       []runOption{withConfig(map[string]any{"releases_url": brokenServer.URL})},
			wantStdout: renderTable(baseInfo),
			wantStderr: "Warning: failed to check for updates: unexpected status code: 404\n",
		},
		{
			// The exit is a bare ExitCodeError (empty message), and the update
			// warning ends with the installation-specific upgrade command,
			// hence the prefix match.
			name: "check with update available",
			args: []string{"version", "--check", "-o", "bare"},
			opts: []runOption{
				withCLIVersion("0.1.0"),
				withConfig(map[string]any{"releases_url": upToDateServer.URL}),
			},
			wantStdout: "0.1.0\n",
			wantErr:    "",
			wantStderr: matchPrefix("\n\nA new release of tiger-cli is available: 0.1.0 → 99.99.99\nTo upgrade: "),
			checks:     []checkFunc{checkExitCode(common.ExitUpdateAvailable)},
		},
	}
	runCmdTests(t, tests)
}
