package cmd

import (
	"testing"
)

func TestConfigUnsetCmd(t *testing.T) {
	tests := []cmdTest{
		{
			name:    "missing argument",
			args:    []string{"config", "unset"},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "too many arguments",
			args:    []string{"config", "unset", "key", "extra"},
			wantErr: "accepts 1 arg(s), received 2",
		},
		{
			name:    "unknown key",
			args:    []string{"config", "unset", "unknown_key"},
			wantErr: "failed to unset config: unknown configuration key: unknown_key",
		},
		{
			name:       "unset removes key and preserves others",
			args:       []string{"config", "unset", "service_id"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "test-service", "output": "json"})},
			wantStdout: "Unset service_id\n",
			check:      checkConfigFile(map[string]any{"output": "json"}),
		},
		{
			name:       "unset valid key not in config file",
			args:       []string{"config", "unset", "analytics"},
			wantStdout: "Unset analytics\n",
			check:      checkConfigFile(map[string]any{}),
		},
		{
			// version_check_interval is a legacy key (not a current config
			// option); unset accepts any key present in the file so stale keys
			// can be removed.
			name:       "unset legacy key present in config file",
			args:       []string{"config", "unset", "version_check_interval"},
			opts:       []runOption{withConfig(map[string]any{"version_check_interval": 0})},
			wantStdout: "Unset version_check_interval\n",
			check:      checkConfigFile(map[string]any{}),
		},
		{
			name:       "rm alias",
			args:       []string{"config", "rm", "service_id"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "test-service"})},
			wantStdout: "Unset service_id\n",
			check:      checkConfigFile(map[string]any{}),
		},
		{
			name:       "delete alias",
			args:       []string{"config", "delete", "output"},
			opts:       []runOption{withConfig(map[string]any{"output": "json"})},
			wantStdout: "Unset output\n",
			check:      checkConfigFile(map[string]any{}),
		},
	}

	runCmdTests(t, tests)
}
