package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func TestDBConnect_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Execute db connect command without service ID
	_, err = executeDBCommand(t.Context(), "db", "connect")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBConnect_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute db connect command
	_, err = executeDBCommand(t.Context(), "db", "connect")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestDBConnect_PsqlNotFound(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Test that psql alias works the same as connect
	_, err1 := executeDBCommand(t.Context(), "db", "connect")
	_, err2 := executeDBCommand(t.Context(), "db", "psql")

	// Both should behave identically (both will fail due to network/psql not found, but with same error pattern)
	if err1 == nil || err2 == nil {
		t.Fatal("Expected both connect and psql to fail in test environment")
	}

	// Both should have similar error patterns (either network error or psql not found)
	connectErrStr := err1.Error()
	psqlErrStr := err2.Error()

	// They should both fail for the same fundamental reason
	if strings.Contains(connectErrStr, "authentication") != strings.Contains(psqlErrStr, "authentication") ||
		strings.Contains(connectErrStr, "psql client not found") != strings.Contains(psqlErrStr, "psql client not found") ||
		strings.Contains(connectErrStr, "failed to fetch") != strings.Contains(psqlErrStr, "failed to fetch") {
		t.Errorf("Connect and psql should behave identically. Connect error: %v, Psql error: %v", err1, err2)
	}
}

func TestSeparateServiceAndPsqlArgs(t *testing.T) {
	testCases := []struct {
		name                string
		args                []string
		argsLenAtDash       int // What ArgsLenAtDash should return
		expectedServiceArgs []string
		expectedPsqlFlags   []string
	}{
		{
			name:                "No separator - service only",
			args:                []string{"svc-12345"},
			argsLenAtDash:       -1, // No -- found
			expectedServiceArgs: []string{"svc-12345"},
			expectedPsqlFlags:   []string{},
		},
		{
			name:                "No arguments at all",
			args:                []string{},
			argsLenAtDash:       -1,
			expectedServiceArgs: []string{},
			expectedPsqlFlags:   []string{},
		},
		{
			name:                "Service with psql flags after --",
			args:                []string{"svc-12345", "-c", "SELECT 1;"},
			argsLenAtDash:       1, // -- was after first arg
			expectedServiceArgs: []string{"svc-12345"},
			expectedPsqlFlags:   []string{"-c", "SELECT 1;"},
		},
		{
			name:                "No service, just psql flags after --",
			args:                []string{"--single-transaction", "--quiet"},
			argsLenAtDash:       0, // -- was at the beginning
			expectedServiceArgs: []string{},
			expectedPsqlFlags:   []string{"--single-transaction", "--quiet"},
		},
		{
			name:                "Service with multiple psql flags",
			args:                []string{"svc-test", "-c", "SELECT version();", "--no-psqlrc", "-v", "ON_ERROR_STOP=1"},
			argsLenAtDash:       1,
			expectedServiceArgs: []string{"svc-test"},
			expectedPsqlFlags:   []string{"-c", "SELECT version();", "--no-psqlrc", "-v", "ON_ERROR_STOP=1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock command that returns the expected ArgsLenAtDash
			mockCmd := &mockCobraCommand{
				args:          tc.args,
				argsLenAtDash: tc.argsLenAtDash,
			}

			serviceArgs, psqlFlags := separateServiceAndPsqlArgs(mockCmd, tc.args)

			if !equalStringSlices(serviceArgs, tc.expectedServiceArgs) {
				t.Errorf("Expected serviceArgs %v, got %v", tc.expectedServiceArgs, serviceArgs)
			}

			if !equalStringSlices(psqlFlags, tc.expectedPsqlFlags) {
				t.Errorf("Expected psqlFlags %v, got %v", tc.expectedPsqlFlags, psqlFlags)
			}
		})
	}
}

// mockCobraCommand implements the minimal interface needed for testing
type mockCobraCommand struct {
	args          []string
	argsLenAtDash int
}

func (m *mockCobraCommand) ArgsLenAtDash() int {
	return m.argsLenAtDash
}

// Helper function to compare string slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func testPrimary() api.Service {
	return api.Service{
		ServiceId: util.Ptr("svc-primary"),
		Name:      util.Ptr("my-db"),
	}
}

func testReplicas() []api.ReadReplicaSet {
	return []api.ReadReplicaSet{
		{Id: util.Ptr("rep-1"), Name: util.Ptr("replica-a")},
		{Id: util.Ptr("rep-2"), Name: util.Ptr("replica-b")},
	}
}

func TestNewConnectTargetModel_Options(t *testing.T) {
	// No replicas: primary, cancel.
	m := newConnectTargetModel(testPrimary(), nil)
	if len(m.choices) != 2 {
		t.Fatalf("expected 2 choices with no replicas, got %d: %v", len(m.choices), m.choices)
	}
	if m.choices[0].kind != targetPrimary {
		t.Errorf("expected first choice to be primary")
	}
	if m.choices[1].kind != targetCancel {
		t.Errorf("expected last choice to be cancel")
	}

	// Two replicas: primary, replica-a, replica-b, cancel.
	m = newConnectTargetModel(testPrimary(), testReplicas())
	if len(m.choices) != 4 {
		t.Fatalf("expected 4 choices with two replicas, got %d: %v", len(m.choices), m.choices)
	}
	if m.choices[1].kind != targetReplica || m.choices[1].replica == nil || *m.choices[1].replica.Id != "rep-1" {
		t.Errorf("expected second choice to be replica rep-1, got %+v", m.choices[1])
	}
	if m.choices[2].kind != targetReplica || *m.choices[2].replica.Id != "rep-2" {
		t.Errorf("expected third choice to be replica rep-2, got %+v", m.choices[2])
	}
	if m.choices[3].kind != targetCancel {
		t.Errorf("expected last choice to be cancel when replicas exist, got %v", m.choices[3].kind)
	}
}

func TestConnectTargetModel_DefaultsToCancel(t *testing.T) {
	m := newConnectTargetModel(testPrimary(), testReplicas())
	if m.chosen.kind != targetCancel {
		t.Errorf("expected default chosen to be cancel, got %v", m.chosen.kind)
	}
}

func TestConnectTargetModel_KeySelection(t *testing.T) {
	cases := []struct {
		name          string
		key           tea.KeyMsg
		wantKind      connectTargetKind
		wantReplicaID string // checked only when set
	}{
		{"q cancels", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, targetCancel, ""},
		{"enter selects primary (cursor starts at 0)", tea.KeyMsg{Type: tea.KeyEnter}, targetPrimary, ""},
		{"'2' selects the first replica", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, targetReplica, "rep-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newConnectTargetModel(testPrimary(), testReplicas())
			updated, _ := m.Update(tc.key)
			choice := updated.(connectTargetModel).chosen
			if choice.kind != tc.wantKind {
				t.Fatalf("expected kind %v, got %v", tc.wantKind, choice.kind)
			}
			if tc.wantReplicaID != "" && (choice.replica == nil || *choice.replica.Id != tc.wantReplicaID) {
				t.Errorf("expected replica %s, got %+v", tc.wantReplicaID, choice.replica)
			}
		})
	}
}

// TestSelectConnection_NoReplicasSkipsPrompt verifies that, with no
// connectable replicas, selectConnection connects to the primary directly
// instead of showing a single-option menu (which would block on TTY input in
// this test).
func TestSelectConnection_NoReplicasSkipsPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) // no replicas
	}))
	defer server.Close()

	client, err := api.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	host := "primary.example.com"
	port := 5432
	primary := api.Service{
		ServiceId: util.Ptr("svc-primary"),
		Name:      util.Ptr("my-db"),
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}

	// Pretend we're on a TTY so the prompt would normally run.
	orig := checkStdinIsTTY
	checkStdinIsTTY = func() bool { return true }
	defer func() { checkStdinIsTTY = orig }()

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	target := &common.ConnectionTarget{ConnectionService: primary, CredentialService: primary}
	cfg := &common.Config{Config: testConfig(t), Client: client, ProjectID: "proj-1"}
	details, err := selectConnection(context.Background(), cmd, cfg, target,
		common.ConnectionDetailsOptions{Role: "tsdbadmin"}, false /*noReplicaPrompt*/)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details == nil || details.Host != host {
		t.Fatalf("expected to connect directly to primary %q, got %+v", host, details)
	}
}

func TestIsAuthenticationError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "PostgreSQL error code 28P01 (invalid_password)",
			err: &pgconn.PgError{
				Code:    "28P01",
				Message: "password authentication failed for user \"test\"",
			},
			expected: true,
		},
		{
			name: "PostgreSQL error code 28000 (invalid_authorization_specification)",
			err: &pgconn.PgError{
				Code:    "28000",
				Message: "role \"nonexistent\" does not exist",
			},
			expected: true,
		},
		{
			name: "PostgreSQL error code 57P03 (cannot_connect_now) - not auth error",
			err: &pgconn.PgError{
				Code:    "57P03",
				Message: "the database system is starting up",
			},
			expected: false,
		},
		{
			name: "PostgreSQL error code 3D000 (database does not exist) - not auth error",
			err: &pgconn.PgError{
				Code:    "3D000",
				Message: "database \"nonexistent\" does not exist",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isAuthenticationError(tc.err)

			if result != tc.expected {
				t.Errorf("Expected isAuthenticationError to return %v for error %v, got %v",
					tc.expected, tc.err, result)
			}
		})
	}
}

func TestLaunchPsqlWithConnectionString(t *testing.T) {
	// This test verifies the psql launching logic without actually running psql

	// Create a test command to capture output
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)

	psqlPath := "/fake/path/to/psql" // This will fail, but we can test the setup

	// Create a dummy service for the test
	service := api.Service{}
	connectionDetails := &common.ConnectionDetails{
		Host:     "testhost",
		Port:     5432,
		Database: "testdb",
		Role:     "testuser",
		Password: "",
	}

	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsql(testConfig(t), connectionDetails, psqlPath, []string{}, service, cmd)

	// Should fail with exec error since fake psql path doesn't exist
	if err == nil {
		t.Error("Expected error when using fake psql path")
	}

	// No output expected since we removed the connecting message
	output := outBuf.String()
	if output != "" {
		t.Errorf("Expected no output, got: %q", output)
	}
}

func TestLaunchPsqlWithAdditionalFlags(t *testing.T) {
	// This test verifies that additional flags are passed correctly to psql

	// Create a test command to capture output
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)

	psqlPath := "/fake/path/to/psql" // This will fail, but we can test the setup
	additionalFlags := []string{"--single-transaction", "--quiet", "-c", "SELECT 1;"}

	// Create a dummy service for the test
	service := api.Service{}

	connectionDetails := &common.ConnectionDetails{
		Host:     "testhost",
		Port:     5432,
		Database: "testdb",
		Role:     "testuser",
		Password: "",
	}

	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsql(testConfig(t), connectionDetails, psqlPath, additionalFlags, service, cmd)

	// Should fail with exec error since fake psql path doesn't exist
	if err == nil {
		t.Error("Expected error when using fake psql path")
	}

	// No output expected since we removed the connecting message
	output := outBuf.String()
	if output != "" {
		t.Errorf("Expected no output, got: %q", output)
	}
}

func TestBuildPsqlCommand_KeyringPasswordEnvVar(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

	// Create a test service
	serviceID := "test-psql-service"
	projectID := "test-psql-project"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	// Store a test password in keyring
	testPassword := "test-password-12345"
	storage := common.GetPasswordStorage(testConfig(t))
	err := storage.Save(service, testPassword, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, "tsdbadmin") // Clean up after test

	psqlPath := "/usr/bin/psql"
	additionalFlags := []string{"--quiet"}

	connectionDetails := &common.ConnectionDetails{
		Host:     "testhost",
		Port:     5432,
		Database: "testdb",
		Role:     "testuser",
		Password: testPassword,
	}

	// Create a mock command for testing
	testCmd := &cobra.Command{}

	// Call the actual production function that builds the command
	psqlCmd := buildPsqlCommand(testConfig(t), connectionDetails, psqlPath, additionalFlags, service, testCmd)

	if psqlCmd == nil {
		t.Fatal("buildPsqlCommand returned nil")
	}

	// Verify that PGPASSWORD is set in the environment with the correct value
	found := false
	expectedEnvVar := "PGPASSWORD=" + testPassword
	for _, envVar := range psqlCmd.Env {
		if envVar == expectedEnvVar {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected PGPASSWORD=%s to be set in environment, but it wasn't. Env vars: %v", testPassword, psqlCmd.Env)
	}
}

func TestBuildPsqlCommand_PgpassStorage_NoEnvVar(t *testing.T) {
	// Set pgpass as the password storage method for this test
	t.Setenv("TIGER_PASSWORD_STORAGE", "pgpass")

	// Create a test service
	serviceID := "test-service-id"
	projectID := "test-project-id"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	psqlPath := "/usr/bin/psql"

	connectionDetails := &common.ConnectionDetails{
		Host:     "testhost",
		Port:     5432,
		Database: "testdb",
		Role:     "testuser",
		Password: "", // Password should be fetched from .pgpass
	}

	// Create a mock command for testing
	testCmd := &cobra.Command{}

	// Call the actual production function that builds the command
	psqlCmd := buildPsqlCommand(testConfig(t), connectionDetails, psqlPath, []string{}, service, testCmd)

	if psqlCmd == nil {
		t.Fatal("buildPsqlCommand returned nil")
	}

	// Verify that PGPASSWORD is NOT set in the environment for pgpass storage
	if psqlCmd.Env != nil {
		for _, envVar := range psqlCmd.Env {
			if strings.HasPrefix(envVar, "PGPASSWORD=") {
				t.Errorf("PGPASSWORD should not be set when using pgpass storage, but found: %s", envVar)
			}
		}
	}
}
