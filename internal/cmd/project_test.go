package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// setupProjectTest sets up the OAuth login tests' mock environment, and stores
// an OAuth login for currentProjectID — pass "" to store nothing.
func setupProjectTest(t *testing.T, projects []api.Project, currentProjectID string) {
	t.Helper()

	setupOAuthTest(t, projects)

	if currentProjectID != "" {
		if err := testConfig(t).StoreOAuthCredentials(testOAuthToken(), currentProjectID); err != nil {
			t.Fatalf("Failed to store oauth credentials: %v", err)
		}
	}
}

// testOAuthToken returns a valid token matching what the mock server mints;
// tests that need a refresh adjust the copy.
func testOAuthToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  "mock-access-token-12345",
		RefreshToken: "mock-refresh-token-67890",
		Expiry:       time.Now().Add(time.Hour),
	}
}

// assertStoredProject checks the stored credentials still hold an OAuth session
// and name the expected project.
func assertStoredProject(t *testing.T, expectedProjectID string) {
	t.Helper()

	stored, err := testConfig(t).GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.OAuth == nil {
		t.Fatalf("Expected OAuth credentials to remain stored, got: %+v", stored)
	}
	if stored.OAuth.AccessToken != "mock-access-token-12345" {
		t.Errorf("Expected access token to be preserved, got %q", stored.OAuth.AccessToken)
	}
	if stored.OAuth.RefreshToken != "mock-refresh-token-67890" {
		t.Errorf("Expected refresh token to be preserved, got %q", stored.OAuth.RefreshToken)
	}
	if stored.ProjectID != expectedProjectID {
		t.Errorf("Expected project ID %q, got %q", expectedProjectID, stored.ProjectID)
	}
}

func TestProject_SwitchByArgument(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")

	output, err := executeAuthCommand(t.Context(), "project", "project-456")
	if err != nil {
		t.Fatalf("Switching projects failed: %v", err)
	}

	expected := "Switched to project Test Project 2 (project-456)\n"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}

	assertStoredProject(t, "project-456")
}

// TestProject_PreservesRefreshedToken covers a token that rotates mid-command:
// listing projects refreshes the expired access token, so the switch must store
// that rotated token rather than the stale copy it read on the way in.
func TestProject_PreservesRefreshedToken(t *testing.T) {
	setupProjectTest(t, testProjects, "")

	expired := testOAuthToken()
	expired.AccessToken = "stale-access-token"
	expired.Expiry = time.Now().Add(-time.Hour)
	if err := testConfig(t).StoreOAuthCredentials(expired, "project-123"); err != nil {
		t.Fatalf("Failed to store oauth credentials: %v", err)
	}

	if _, err := executeAuthCommand(t.Context(), "project", "project-456"); err != nil {
		t.Fatalf("Switching projects failed: %v", err)
	}

	// assertStoredProject expects the refreshed token, so this fails if the
	// stale access token was written back.
	assertStoredProject(t, "project-456")
}

// TestProject_SurvivesPostSwitchRefresh covers a refresh that happens after the
// switch is stored: the API client persists rotated tokens under the project ID
// it was built for, so it has to be rebuilt or the refresh writes the old
// project back.
func TestProject_SurvivesPostSwitchRefresh(t *testing.T) {
	setupProjectTest(t, testProjects, "")

	// Every mint refreshes and rotates the token, so any request persists one,
	// and analytics fires one after the switch is stored.
	mockShortLivedRotatingTokens(t)
	t.Setenv("TIGER_ANALYTICS", "true")

	expired := testOAuthToken()
	expired.AccessToken = "stale-access-token"
	expired.Expiry = time.Now().Add(-time.Hour)
	if err := testConfig(t).StoreOAuthCredentials(expired, "project-123"); err != nil {
		t.Fatalf("Failed to store oauth credentials: %v", err)
	}

	if _, err := executeAuthCommand(t.Context(), "project", "project-456"); err != nil {
		t.Fatalf("Switching projects failed: %v", err)
	}

	stored, err := testConfig(t).GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.ProjectID != "project-456" {
		t.Errorf("Expected the switch to survive a later token refresh, got project %q", stored.ProjectID)
	}
}

func TestProject_ClearsDefaultService(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")

	if err := testConfig(t).Set("service_id", "svc1234567"); err != nil {
		t.Fatalf("Failed to set default service: %v", err)
	}

	output, err := executeAuthCommand(t.Context(), "project", "project-789")
	if err != nil {
		t.Fatalf("Switching projects failed: %v", err)
	}

	if !strings.Contains(output, "Cleared default service 'svc1234567'") {
		t.Errorf("Expected output to report the cleared default service, got %q", output)
	}
	assertStoredProject(t, "project-789")

	cfg, err := config.LoadForOutput(testConfigDir(t), false, true)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.ServiceID != nil {
		t.Errorf("Expected service_id to be removed from the config file, got %q", *cfg.ServiceID)
	}
}

// TestProject_DefaultServiceFromEnvironment covers a default service that Unset
// can't remove, because it comes from the environment rather than the config
// file: the command must not claim to have cleared it.
func TestProject_DefaultServiceFromEnvironment(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")
	t.Setenv("TIGER_SERVICE_ID", "envsvc1234")

	output, err := executeAuthCommand(t.Context(), "project", "project-456")
	if err != nil {
		t.Fatalf("Switching projects failed: %v", err)
	}

	if strings.Contains(output, "Cleared default service") {
		t.Errorf("Expected no claim of clearing an environment default, got %q", output)
	}
	if !strings.Contains(output, "Default service 'envsvc1234' comes from a flag or environment variable") {
		t.Errorf("Expected a warning that the default service is unchanged, got %q", output)
	}
}

func TestProject_AlreadyCurrent(t *testing.T) {
	setupProjectTest(t, testProjects, "project-456")

	output, err := executeAuthCommand(t.Context(), "project", "project-456")
	if err != nil {
		t.Fatalf("Expected success for a no-op switch, got: %v", err)
	}

	expected := "Already using project Test Project 2 (project-456)\n"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}

	assertStoredProject(t, "project-456")
}

func TestProject_UnknownProject(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")

	_, err := executeAuthCommand(t.Context(), "project", "project-does-not-exist")
	if err == nil {
		t.Fatal("Expected switching to an inaccessible project to fail")
	}
	if !strings.Contains(err.Error(), `project "project-does-not-exist" not found or not accessible`) {
		t.Errorf("Unexpected error: %v", err)
	}

	assertExitCode(t, err, common.ExitInvalidParameters)

	// The stored project is unchanged
	assertStoredProject(t, "project-123")
}

func TestProject_SelectsInteractively(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")

	// The picker only runs on a TTY
	stubIsTerminal(t, true)

	stubSelectProject(t, func(_ *cobra.Command, projects []api.Project) (api.Project, error) {
		if len(projects) != len(testProjects) {
			t.Errorf("Expected %d projects in the picker, got %d", len(testProjects), len(projects))
		}
		return projects[2], nil
	})

	output, err := executeAuthCommand(t.Context(), "project")
	if err != nil {
		t.Fatalf("Switching projects failed: %v", err)
	}

	expected := "Switched to project Test Project 3 (project-789)\n"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}

	assertStoredProject(t, "project-789")
}

func TestProject_NoArgumentWithoutTTY(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")

	stubIsTerminal(t, false)

	_, err := executeAuthCommand(t.Context(), "project")
	if err == nil {
		t.Fatal("Expected a bare switch to fail without a TTY")
	}
	if !strings.Contains(err.Error(), "pass the project ID as an argument") {
		t.Errorf("Expected the error to name the non-interactive alternative, got: %v", err)
	}

	assertStoredProject(t, "project-123")
}

func TestProject_SingleProject(t *testing.T) {
	setupProjectTest(t, testProjects[:1], "project-123")

	// No prompt is needed to pick from one project, so this works without a TTY
	stubIsTerminal(t, false)

	output, err := executeAuthCommand(t.Context(), "project")
	if err != nil {
		t.Fatalf("Expected success with a single project, got: %v", err)
	}

	expected := "Already using project Test Project 1 (project-123)\n"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}
}

func TestProject_RejectsAPIKeyCredentials(t *testing.T) {
	setupProjectTest(t, testProjects, "")

	if err := testConfig(t).StoreCredentials("public:secret", "project-123"); err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	_, err := executeAuthCommand(t.Context(), "project", "project-456")
	if err == nil {
		t.Fatal("Expected switching projects to fail for an API key login")
	}
	if !strings.Contains(err.Error(), "switching projects requires an OAuth login") {
		t.Errorf("Unexpected error: %v", err)
	}

	// The API key login is left untouched
	stored, err := testConfig(t).GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.APIKey != "public:secret" || stored.ProjectID != "project-123" {
		t.Errorf("Expected stored API key credentials to be unchanged, got %+v", stored)
	}
}

func TestProject_RejectsEnvironmentAPIKeys(t *testing.T) {
	setupProjectTest(t, testProjects, "project-123")

	// These take precedence over the stored login, so a switch wouldn't stick.
	t.Setenv("TIGER_PUBLIC_KEY", "env-public-key")
	t.Setenv("TIGER_SECRET_KEY", "env-secret-key")

	_, err := executeAuthCommand(t.Context(), "project", "project-456")
	if err == nil {
		t.Fatal("Expected switching projects to fail with API keys in the environment")
	}
	if !strings.Contains(err.Error(), "cannot switch projects while TIGER_PUBLIC_KEY/TIGER_SECRET_KEY are set") {
		t.Errorf("Unexpected error: %v", err)
	}

	assertStoredProject(t, "project-123")
}

func TestProject_NotLoggedIn(t *testing.T) {
	setupProjectTest(t, testProjects, "")

	_, err := executeAuthCommand(t.Context(), "project", "project-456")
	if err == nil {
		t.Fatal("Expected switching projects to fail when not logged in")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Unexpected error: %v", err)
	}

	assertExitCode(t, err, common.ExitAuthenticationError)
}
