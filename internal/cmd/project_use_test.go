package cmd

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// setupProjectTest prepares an OAuth login against a mock API server that
// serves the given projects, with currentProjectID as the active project and
// an optional default service.
func setupProjectTest(t *testing.T, projects []api.Project, currentProjectID, serviceID string) {
	t.Helper()
	setupOAuthTest(t, projects, currentProjectID)

	cfg := testConfig(t)
	if serviceID != "" {
		if err := cfg.Set("service_id", serviceID); err != nil {
			t.Fatalf("Failed to set service_id: %v", err)
		}
	}

	token := &oauth2.Token{
		AccessToken:  "valid-access-token",
		RefreshToken: "valid-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}
	if err := cfg.StoreOAuthCredentials(token, currentProjectID); err != nil {
		t.Fatalf("Failed to store OAuth credentials: %v", err)
	}
}

func TestProjectUse_Switch(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
		{ID: "project-new", Name: "New Project"},
	}, "project-old", "svc-123")

	output, err := executeAuthCommand(t.Context(), "project", "use", "project-new")
	if err != nil {
		t.Fatalf("Switch failed: %v", err)
	}

	expectedOutput := "Cleared default service (config key service_id): it belonged to the previous project\n" +
		"Switched to project project-new\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: %q", output)
	}

	stored, err := testConfig(t).GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.ProjectID != "project-new" {
		t.Errorf("Expected project ID 'project-new', got %q", stored.ProjectID)
	}
	if stored.OAuth == nil || stored.OAuth.AccessToken != "valid-access-token" {
		t.Errorf("Expected OAuth token to be preserved, got %+v", stored.OAuth)
	}

	if serviceID := testConfig(t).ServiceID; serviceID != "" {
		t.Errorf("Expected service_id to be cleared, got %q", serviceID)
	}
}

func TestProjectUse_SameProject(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
	}, "project-old", "svc-123")

	output, err := executeAuthCommand(t.Context(), "project", "use", "project-old")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	if output != "Already using project project-old\n" {
		t.Errorf("Unexpected output: %q", output)
	}

	// A no-op switch keeps the default service.
	if serviceID := testConfig(t).ServiceID; serviceID != "svc-123" {
		t.Errorf("Expected service_id to be kept, got %q", serviceID)
	}
}

func TestProjectUse_NoAccess(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
	}, "project-old", "")

	output, err := executeAuthCommand(t.Context(), "project", "use", "project-unknown")
	if err == nil {
		t.Fatal("Expected error for inaccessible project")
	}
	if !strings.Contains(err.Error(), "no access to the requested project") {
		t.Errorf("Unexpected error: %v", err)
	}
	// The requested ID is echoed on stderr, not in the error.
	if !strings.Contains(output, "Project project-unknown is not among your accessible projects") {
		t.Errorf("Expected stderr to name the project, got: %q", output)
	}
	assertExitCode(t, err, common.ExitInvalidParameters)

	stored, err := testConfig(t).GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.ProjectID != "project-old" {
		t.Errorf("Expected project ID to stay 'project-old', got %q", stored.ProjectID)
	}
}

func TestProjectUse_APIKeyLogin(t *testing.T) {
	setupAuthTest(t)

	if err := testConfig(t).StoreCredentials("pub:sec", "project-old"); err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	_, err := executeAuthCommand(t.Context(), "project", "use", "project-new")
	if err == nil {
		t.Fatal("Expected error for API key login")
	}
	if !strings.Contains(err.Error(), "an API key is scoped to a single project") {
		t.Errorf("Unexpected error: %v", err)
	}
	assertExitCode(t, err, common.ExitAuthenticationError)
}

func TestProjectUse_EnvAPIKeys(t *testing.T) {
	tmpDir := setupAuthTest(t)
	// Keep the env-key validation in wrapCommands off the network.
	if _, err := config.UseTestConfig(tmpDir, map[string]any{"api_url": "http://localhost:1"}); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	t.Setenv("TIGER_PUBLIC_KEY", "env-public")
	t.Setenv("TIGER_SECRET_KEY", "env-secret")

	_, err := executeAuthCommand(t.Context(), "project", "use", "project-new")
	if err == nil {
		t.Fatal("Expected error with env API keys set")
	}
	if !strings.Contains(err.Error(), "cannot switch projects while TIGER_PUBLIC_KEY/TIGER_SECRET_KEY are set") {
		t.Errorf("Unexpected error: %v", err)
	}
	assertExitCode(t, err, common.ExitAuthenticationError)
}

func TestProjectUse_EnvServiceID(t *testing.T) {
	setupProjectTest(t, []api.Project{
		{ID: "project-old", Name: "Old Project"},
		{ID: "project-new", Name: "New Project"},
	}, "project-old", "")
	t.Setenv("TIGER_SERVICE_ID", "svc-env")

	output, err := executeAuthCommand(t.Context(), "project", "use", "project-new")
	if err != nil {
		t.Fatalf("Switch failed: %v", err)
	}

	// The env-provided default can't be cleared, so the command must warn
	// instead of claiming it was cleared.
	expectedOutput := "Warning: the default service from --service-id/TIGER_SERVICE_ID belongs to the previous project and is still in effect\n" +
		"Switched to project project-new\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: %q", output)
	}
}

func TestProjectUse_NotLoggedIn(t *testing.T) {
	setupAuthTest(t)

	_, err := executeAuthCommand(t.Context(), "project", "use", "project-new")
	if err == nil {
		t.Fatal("Expected error when not logged in")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Unexpected error: %v", err)
	}
}
