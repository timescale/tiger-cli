package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestConfigResetCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:    "unexpected argument",
			args:    []string{"config", "reset", "extra"},
			wantErr: `unknown command "extra" for "tiger config reset"`,
		},
		{
			name: "reset clears configured values",
			args: []string{"config", "reset"},
			opts: []runOption{withConfig(map[string]any{
				"service_id": "custom-service",
				"output":     "json",
				"analytics":  false,
			})},
			wantStdout: "Configuration reset to defaults\n",
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				// Reset empties the config file rather than writing defaults
				// into it, so env vars still apply afterwards.
				contents, err := os.ReadFile(config.GetConfigFile(result.configDir))
				if err != nil {
					t.Fatalf("failed to read config file: %v", err)
				}
				if strings.TrimSpace(string(contents)) != "{}" {
					t.Errorf("expected an empty config file, got %q", string(contents))
				}
			}},
		},
		{
			name:       "clear alias",
			args:       []string{"config", "clear"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "custom-service"})},
			wantStdout: "Configuration reset to defaults\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{})},
		},
	})
}
