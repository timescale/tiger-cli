package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

func TestDBSchema_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)

	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	mockTestPAT(t)

	_, err = executeDBCommand(t.Context(), "db", "schema")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBSchema_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)

	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	mockNotLoggedIn(t)

	_, err = executeDBCommand(t.Context(), "db", "schema")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

// withMockService overrides getServiceDetailsFunc to return the given service,
// restoring the original when the test ends.
func withMockService(t *testing.T, service api.Service) {
	t.Helper()
	original := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return service, nil
	}
	t.Cleanup(func() { getServiceDetailsFunc = original })
}

// TestDBSchema_NotReadyStates checks that the command surfaces the readiness
// guard's error before any connection attempt. The full status->error matrix is
// covered by TestCheckServiceReady; here we only verify it propagates.
func TestDBSchema_NotReadyStates(t *testing.T) {
	tests := []struct {
		name    string
		status  api.DeployStatus
		wantMsg string
	}{
		{"paused", api.PAUSED, "service is paused"},
		{"not ready", api.QUEUED, "service is not ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := setupDBTest(t)
			if _, err := config.UseTestConfig(tmpDir, map[string]any{
				"api_url":    "https://api.tigerdata.com/public/v1",
				"service_id": "svc-12345",
			}); err != nil {
				t.Fatalf("Failed to save test config: %v", err)
			}

			mockTestPAT(t)
			withMockService(t, api.Service{
				ServiceId: util.Ptr("svc-12345"),
				Status:    util.Ptr(tt.status),
			})

			_, err := executeDBCommand(t.Context(), "db", "schema")
			if err == nil {
				t.Fatalf("Expected error for a %s service", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Expected %q, got: %v", tt.wantMsg, err)
			}
		})
	}
}
