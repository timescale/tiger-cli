package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

// checkConfig returns a check asserting against the config the invocation
// actually resolved (see cmdResult.cfg).
func checkConfig(assert func(t *testing.T, cfg *config.Config)) checkFunc {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		if result.cfg == nil {
			t.Fatal("command did not load a config")
		}
		assert(t, result.cfg)
	}
}

// checkConfigValue returns a check asserting one config field, named for the
// message. Use it with a getter: checkConfigValue("service_id", ...).
func checkConfigValue(key string, get func(*config.Config) string, want string) checkFunc {
	return checkConfig(func(t *testing.T, cfg *config.Config) {
		t.Helper()
		if got := get(cfg); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	})
}

func serviceID(cfg *config.Config) string { return cfg.ServiceID }
func output(cfg *config.Config) string    { return cfg.Output }
func apiURL(cfg *config.Config) string    { return cfg.APIURL }

func TestRootCmd(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	runCmdTests(t, []cmdTest{
		{
			// The context passed to buildRootCmd must reach the command that
			// runs, so handlers can rely on cmd.Context() for cancellation.
			// Observed through `version --check`: its HTTP fetch runs under
			// cmd.Context(), so an already-cancelled context fails the check
			// (and only the check).
			name: "context reaches command",
			args: []string{"version", "--check", "-o", "bare"},
			opts: []runOption{
				withContext(cancelledCtx),
				withConfig(map[string]any{"releases_url": server.URL}),
			},
			wantStdout: config.Version + "\n",
			wantStderr: fmt.Sprintf(
				"Warning: failed to check for updates: failed to fetch latest version: Get %q: context canceled\n",
				server.URL+"/latest.txt"),
		},
		{
			// Command names and flags match case-insensitively
			// (cobra.EnableCaseInsensitive and the flag normalization func,
			// both configured in buildRootCmd).
			name:       "case-insensitive commands and flags",
			args:       []string{"VERSION", "--Output", "bare"},
			wantStdout: config.Version + "\n",
		},
	})

	// Config precedence is flag > env > file > default. Each case runs a
	// command and asserts against the config that invocation resolved, so it
	// tests the precedence the command itself saw rather than a re-derivation.
	// `version` is the command under test throughout: it needs no API calls,
	// and it defines --output, so the flag binding for that key is live.
	t.Run("config precedence", func(t *testing.T) {
		runCmdTests(t, []cmdTest{
			{
				name: "flag beats env var and config file",
				args: []string{"--service-id", "flag-service", "version", "-o", "bare"},
				opts: []runOption{
					withConfig(map[string]any{
						"service_id": "file-service",
						"api_url":    "https://file.api.com/v1",
						"output":     "table",
					}),
					withEnv("TIGER_SERVICE_ID", "env-service"),
					withEnv("TIGER_OUTPUT", "json"),
				},
				wantStdout: config.Version + "\n",
				checks: []checkFunc{
					checkConfigValue("service_id", serviceID, "flag-service"),
					// The --output flag beats the env var, which beats the file.
					checkConfigValue("output", output, "bare"),
					// The file wins where neither a flag nor an env var was given.
					checkConfigValue("api_url", apiURL, "https://file.api.com/v1"),
				},
			},
			{
				name:       "env var beats config file",
				args:       []string{"version"},
				opts:       []runOption{withConfig(map[string]any{"output": "table"}), withEnv("TIGER_OUTPUT", "json")},
				wantStdout: matchPrefix(`{`), // json, from the env var
				checks:     []checkFunc{checkConfigValue("output", output, "json")},
			},
			{
				name:       "env var applies when no flag is given",
				args:       []string{"version", "-o", "bare"},
				opts:       []runOption{withEnv("TIGER_SERVICE_ID", "env-service")},
				wantStdout: config.Version + "\n",
				checks:     []checkFunc{checkConfigValue("service_id", serviceID, "env-service")},
			},
			{
				name:       "config file beats default",
				args:       []string{"version", "-o", "bare"},
				opts:       []runOption{withConfig(map[string]any{"api_url": "https://file.api.com/v1"})},
				wantStdout: config.Version + "\n",
				checks:     []checkFunc{checkConfigValue("api_url", apiURL, "https://file.api.com/v1")},
			},
			{
				// No -o here: the point is that output falls back to its
				// default, which passing the flag would defeat. The table
				// itself is asserted exactly in TestVersionCmd.
				name:       "defaults apply when nothing is set",
				args:       []string{"version"},
				wantStdout: matchPrefix("┌"),
				checks: []checkFunc{
					checkConfigValue("output", output, config.DefaultOutput),
					checkConfigValue("api_url", apiURL, config.DefaultAPIURL),
					checkConfigValue("service_id", serviceID, ""),
				},
			},
		})
	})

	// Only the flags a command actually defines are bound, so a command without
	// an --output flag still resolves output from the env and config file.
	// Asserted on an unexecuted command, since the point is that binding is
	// per-flag-set rather than global.
	t.Run("flag binding is per command", func(t *testing.T) {
		dir := t.TempDir()
		writeConfigFile(t, dir, map[string]any{"output": "yaml"})
		t.Setenv("TIGER_CONFIG_DIR", dir)

		root, _, err := buildRootCmd(t.Context())
		if err != nil {
			t.Fatalf("buildRootCmd failed: %v", err)
		}
		noOutputCmd, _, err := root.Find([]string{"config", "unset"})
		if err != nil {
			t.Fatalf("failed to find command: %v", err)
		}
		if noOutputCmd.Flags().Lookup("output") != nil {
			t.Fatal("expected `config unset` to have no --output flag")
		}

		cfg, err := config.Load(noOutputCmd.Flags())
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.Output != "yaml" {
			t.Errorf("output = %q, want %q from config file", cfg.Output, "yaml")
		}
	})
}
