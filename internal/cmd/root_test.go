package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

// effectiveConfig executes args through the real root command against the given
// config dir, then loads the config the same way the executed command's
// lifecycle did — from the leaf command's flag set — so it reflects the
// flag > env > config-file precedence the command saw.
func effectiveConfig(t *testing.T, configDir string, args ...string) *config.Config {
	t.Helper()
	config.SetTestServiceName(t)

	root, _, err := buildRootCmd(t.Context())
	if err != nil {
		t.Fatalf("buildRootCmd failed: %v", err)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	full := append([]string{"--config-dir", configDir, "--analytics=false", "--skip-update-check"}, args...)
	root.SetArgs(full)
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	leaf, _, err := root.Find(full)
	if err != nil {
		t.Fatalf("failed to find executed command: %v", err)
	}
	cfg, err := config.Load(leaf.Flags())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	return cfg
}

func TestRootCmd(t *testing.T) {
	// The context passed to buildRootCmd must reach the command that runs, so
	// handlers can rely on cmd.Context() for cancellation. Observed through
	// `version --check`: its HTTP fetch runs under cmd.Context(), so an
	// already-cancelled context fails the check (and only the check).
	t.Run("context reaches command", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(server.Close)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := runCommand(t, []string{"version", "--check", "-o", "bare"}, nil,
			withContext(ctx),
			withConfig(map[string]any{"releases_url": server.URL}),
		)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, config.Version+"\n")
		assertOutput(t, result.stderr, fmt.Sprintf(
			"Warning: failed to check for updates: failed to fetch latest version: Get %q: context canceled\n",
			server.URL+"/latest.txt"))
	})

	t.Run("config precedence", func(t *testing.T) {
		tests := []struct {
			name   string
			config map[string]any
			env    map[string]string
			args   []string
			check  func(t *testing.T, cfg *config.Config)
		}{
			{
				name: "flag beats env var and config file",
				config: map[string]any{
					"service_id": "file-service",
					"api_url":    "https://file.api.com/v1",
					"output":     "table",
				},
				env: map[string]string{
					"TIGER_SERVICE_ID": "env-service",
					"TIGER_OUTPUT":     "json",
				},
				args: []string{"--service-id", "flag-service", "version"},
				check: func(t *testing.T, cfg *config.Config) {
					if cfg.ServiceID != "flag-service" {
						t.Errorf("service_id = %q, want %q from flag", cfg.ServiceID, "flag-service")
					}
					// Env var wins where no flag was given
					if cfg.Output != "json" {
						t.Errorf("output = %q, want %q from env var", cfg.Output, "json")
					}
					// Config file wins where neither a flag nor an env var was given
					if cfg.APIURL != "https://file.api.com/v1" {
						t.Errorf("api_url = %q, want value from config file", cfg.APIURL)
					}
				},
			},
			{
				name: "env var applies when no flag is given",
				env:  map[string]string{"TIGER_SERVICE_ID": "env-service"},
				args: []string{"version"},
				check: func(t *testing.T, cfg *config.Config) {
					if cfg.ServiceID != "env-service" {
						t.Errorf("service_id = %q, want %q from env var", cfg.ServiceID, "env-service")
					}
				},
			},
			{
				name: "flag overrides env var",
				env:  map[string]string{"TIGER_SERVICE_ID": "env-service"},
				args: []string{"--service-id", "flag-service", "version"},
				check: func(t *testing.T, cfg *config.Config) {
					if cfg.ServiceID != "flag-service" {
						t.Errorf("service_id = %q, want %q from flag", cfg.ServiceID, "flag-service")
					}
				},
			},
			{
				name: "config file beats default",
				config: map[string]any{
					"output":  "json",
					"api_url": "https://file.api.com/v1",
				},
				args: []string{"version"},
				check: func(t *testing.T, cfg *config.Config) {
					if cfg.Output != "json" {
						t.Errorf("output = %q, want %q from config file", cfg.Output, "json")
					}
					if cfg.APIURL != "https://file.api.com/v1" {
						t.Errorf("api_url = %q, want value from config file", cfg.APIURL)
					}
				},
			},
			{
				name: "defaults apply when nothing is set",
				args: []string{"version"},
				check: func(t *testing.T, cfg *config.Config) {
					if cfg.Output != config.DefaultOutput {
						t.Errorf("output = %q, want default %q", cfg.Output, config.DefaultOutput)
					}
					if cfg.APIURL != config.DefaultAPIURL {
						t.Errorf("api_url = %q, want default %q", cfg.APIURL, config.DefaultAPIURL)
					}
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				dir := t.TempDir()
				if tt.config != nil {
					if _, err := config.UseTestConfig(dir, tt.config); err != nil {
						t.Fatalf("failed to seed config file: %v", err)
					}
				}
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
				tt.check(t, effectiveConfig(t, dir, tt.args...))
			})
		}
	})

	// Only the flags a command actually defines are bound, so a command without
	// an --output flag still resolves output from the env and config file.
	t.Run("flag binding is per command", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := config.UseTestConfig(dir, map[string]any{"output": "yaml"}); err != nil {
			t.Fatalf("failed to seed config file: %v", err)
		}
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
