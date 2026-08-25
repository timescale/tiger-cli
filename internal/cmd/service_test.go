package cmd

import (
	"net/http"
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceCommandAliases(t *testing.T) {
	emptyList := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
			Return(&api.GetServicesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &[]api.Service{},
			}, nil)
	}

	tests := []cmdTest{
		{
			name:       "service",
			args:       []string{"service", "list"},
			setup:      emptyList,
			wantStderr: noServicesStderr,
		},
		{
			name:       "services alias",
			args:       []string{"services", "list"},
			setup:      emptyList,
			wantStderr: noServicesStderr,
		},
		{
			name:       "svc alias",
			args:       []string{"svc", "list"},
			setup:      emptyList,
			wantStderr: noServicesStderr,
		},
	}

	runCmdTests(t, tests)
}

func parseConfigFile(t *testing.T, configFile string) map[string]any {
	t.Helper()

	// Read the config file directly
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	var configMap map[string]any
	if err := yaml.Unmarshal(configBytes, &configMap); err != nil {
		t.Fatalf("Failed to parse config YAML: %v", err)
	}
	return configMap
}
