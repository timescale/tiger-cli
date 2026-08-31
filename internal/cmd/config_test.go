package cmd

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// checkConfigFile returns a check func asserting that the persisted config
// file contains exactly the given keys and values.
func checkConfigFile(want map[string]any) checkFunc {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		got := readConfigFile(t, result.configDir)
		if got == nil {
			got = map[string]any{}
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("config file mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestConfigCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "cfg alias",
			args:       []string{"cfg", "set", "service_id", "alias-service"},
			wantStdout: "Set service_id = alias-service\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"service_id": "alias-service"})},
		},
	})
}
