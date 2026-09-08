package cmd

import (
	"bytes"
	"errors"
	"github.com/zalando/go-keyring"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestDbConnectCmd(t *testing.T) {
	// A stub psql on PATH lets cases get past the LookPath check; every case
	// that does so still fails before psql would actually run.
	psqlDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(psqlDir, "psql"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to create stub psql: %v", err)
	}

	noEndpoint := func(s *api.Service) { s.Endpoint = nil }

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "connect", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "service ID required",
			args:    []string{"db", "connect"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "psql alias",
			args:    []string{"db", "psql"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "args after -- are not the service ID",
			args:    []string{"db", "connect", "--", "--single-transaction"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "default service ID from config with psql flags after --",
			args: []string{"db", "connect", "--", "-c", "SELECT 1;"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "service ID before -- separator",
			args: []string{"db", "connect", "svc-12345", "--", "-c", "SELECT 1;"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error fetching service",
			args: []string{"db", "connect", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"db", "connect", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "parent fetch fails for read replica",
			args: []string{"db", "connect", "rep-67890"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: `failed to fetch parent service "svc-12345" for read replica: failed to fetch service details: connection refused`,
		},
		{
			name: "psql not found",
			args: []string{"db", "connect", "svc-12345"},
			opts: []runOption{withEnv("PATH", "/nonexistent")},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService())
			},
			wantErr: "psql client not found. Please install PostgreSQL client tools",
		},
		{
			// No GetReplicaSets expectation: a non-TTY stdin/stderr must skip
			// the replica prompt entirely.
			name: "non-TTY skips replica prompt",
			args: []string{"db", "connect", "svc-12345"},
			opts: []runOption{withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(noEndpoint))
			},
			wantErr: "failed to build connection string: service endpoint not available",
		},
		{
			name: "--no-replica-prompt skips replica prompt on a TTY",
			args: []string{"db", "connect", "svc-12345", "--no-replica-prompt"},
			opts: []runOption{withIsTerminal(true), withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(noEndpoint))
			},
			wantErr: "failed to build connection string: service endpoint not available",
		},
		{
			name: "replica target skips replica prompt",
			args: []string{"db", "connect", "rep-67890"},
			opts: []runOption{withIsTerminal(true), withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica(noEndpoint))
				expectGetService(m, "svc-12345", sampleService())
			},
			wantErr: "failed to build connection string: service endpoint not available",
		},
		{
			name: "replica listing failure warns and continues",
			args: []string{"db", "connect", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(noEndpoint))
				m.EXPECT().GetReplicaSetsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr:    "failed to build connection string: service endpoint not available",
			wantStderr: "Warning: could not list read replicas: connection refused\nError: failed to build connection string: service endpoint not available\n",
		},
		{
			name: "no replicas skips prompt on a TTY",
			args: []string{"db", "connect", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(noEndpoint))
				m.EXPECT().GetReplicaSetsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetReplicaSetsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &[]api.ReadReplicaSet{},
					}, nil)
			},
			wantErr: "failed to build connection string: service endpoint not available",
		},
		{
			name: "no connectable replicas skips prompt",
			args: []string{"db", "connect", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(noEndpoint))
				replicas := []api.ReadReplicaSet{
					{
						ID:     "rep-1",
						Name:   "replica-a",
						Status: api.ReadReplicaSetStatusCreating,
						Endpoint: &api.Endpoint{
							Host: new("rep.example.com"),
							Port: new(5432),
						},
					},
					{ID: "rep-2", Name: "replica-b", Status: api.ReadReplicaSetStatusActive},
				}
				m.EXPECT().GetReplicaSetsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetReplicaSetsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &replicas,
					}, nil)
			},
			wantErr: "failed to build connection string: service endpoint not available",
		},
		{
			name: "--pooled without pooler",
			args: []string{"db", "connect", "svc-12345", "--pooled"},
			opts: []runOption{withEnv("PATH", psqlDir)},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService())
			},
			wantErr: "connection pooler not available for this service",
		},
		{
			// The stored-password test connection runs before psql launches. A
			// refused connection isn't an auth error, so it must surface
			// directly instead of opening the password recovery menu. No
			// password is stored, so the lookup warning comes first; the pgx
			// error text is environment-dependent, hence the non-exact matches.
			name: "non-auth connection error surfaces directly",
			args: []string{"db", "connect", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("127.0.0.1"), Port: new(1)}
				}))
			},
			opts: []runOption{withEnv("PATH", psqlDir)},
			wantErr: matchFunc(func(t *testing.T, got string) {
				if !strings.Contains(got, "127.0.0.1") {
					t.Errorf("error = %q, want it to name the unreachable host", got)
				}
			}),
			wantStderr: matchPrefix("Warning: could not retrieve stored password: secret not found in keyring\n"),
		},
	})
}

// psqlArgsLenAtDash implements ArgsLenAtDashProvider for
// TestSeparateServiceAndPsqlArgs.
type psqlArgsLenAtDash int

func (p psqlArgsLenAtDash) ArgsLenAtDash() int { return int(p) }

// TestSeparateServiceAndPsqlArgs covers the service-arg/psql-flag split at the
// helper level: the full split isn't observable through the command without
// actually launching psql.
func TestSeparateServiceAndPsqlArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		argsLenAtDash   int
		wantServiceArgs []string
		wantPsqlFlags   []string
	}{
		{"no separator", []string{"svc-12345"}, -1, []string{"svc-12345"}, []string{}},
		{"no arguments", []string{}, -1, []string{}, []string{}},
		{"service with psql flags", []string{"svc-12345", "-c", "SELECT 1;"}, 1, []string{"svc-12345"}, []string{"-c", "SELECT 1;"}},
		{"only psql flags", []string{"--single-transaction", "--quiet"}, 0, []string{}, []string{"--single-transaction", "--quiet"}},
		{"multiple psql flags", []string{"svc-test", "-c", "SELECT version();", "--no-psqlrc", "-v", "ON_ERROR_STOP=1"}, 1, []string{"svc-test"}, []string{"-c", "SELECT version();", "--no-psqlrc", "-v", "ON_ERROR_STOP=1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceArgs, psqlFlags := separateServiceAndPsqlArgs(psqlArgsLenAtDash(tt.argsLenAtDash), tt.args)
			if !slices.Equal(serviceArgs, tt.wantServiceArgs) {
				t.Errorf("expected serviceArgs %v, got %v", tt.wantServiceArgs, serviceArgs)
			}
			if !slices.Equal(psqlFlags, tt.wantPsqlFlags) {
				t.Errorf("expected psqlFlags %v, got %v", tt.wantPsqlFlags, psqlFlags)
			}
		})
	}
}

// TestConnectTargetModel covers the replica-selection menu model at the helper
// level: driving the Bubble Tea menu through the command would need a real TTY.
func TestConnectTargetModel(t *testing.T) {
	primary := api.Service{ServiceID: "svc-primary", Name: "my-db"}
	replicas := []api.ReadReplicaSet{
		{ID: "rep-1", Name: "replica-a"},
		{ID: "rep-2", Name: "replica-b"},
	}

	t.Run("choices", func(t *testing.T) {
		// No replicas: primary, cancel.
		m := newConnectTargetModel(primary, nil)
		if len(m.choices) != 2 || m.choices[0].kind != targetPrimary || m.choices[1].kind != targetCancel {
			t.Errorf("expected [primary, cancel] with no replicas, got %+v", m.choices)
		}

		// Two replicas: primary, replica-a, replica-b, cancel.
		m = newConnectTargetModel(primary, replicas)
		if len(m.choices) != 4 {
			t.Fatalf("expected 4 choices with two replicas, got %d: %+v", len(m.choices), m.choices)
		}
		if m.choices[1].kind != targetReplica || m.choices[1].replica == nil || m.choices[1].replica.ID != "rep-1" {
			t.Errorf("expected second choice to be replica rep-1, got %+v", m.choices[1])
		}
		if m.choices[2].kind != targetReplica || m.choices[2].replica.ID != "rep-2" {
			t.Errorf("expected third choice to be replica rep-2, got %+v", m.choices[2])
		}
		if m.choices[3].kind != targetCancel {
			t.Errorf("expected last choice to be cancel, got %v", m.choices[3].kind)
		}

		// Quitting without a selection must be a no-op connection.
		if m.chosen.kind != targetCancel {
			t.Errorf("expected default chosen to be cancel, got %v", m.chosen.kind)
		}
	})

	// Ctrl+C is {Code: 'c', Mod: tea.ModCtrl}; the raw control byte {Code: 3}
	// stringifies to "\x03" and would match nothing.
	keyTests := []struct {
		name          string
		key           tea.KeyPressMsg
		wantKind      connectTargetKind
		wantReplicaID string // checked only when set
	}{
		{"q cancels", tea.KeyPressMsg{Code: 'q'}, targetCancel, ""},
		{"enter selects primary (cursor starts at 0)", tea.KeyPressMsg{Code: tea.KeyEnter}, targetPrimary, ""},
		{"space selects primary", tea.KeyPressMsg{Code: tea.KeySpace}, targetPrimary, ""},
		{"ctrl+c cancels", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, targetCancel, ""},
		{"'2' selects the first replica", tea.KeyPressMsg{Code: '2'}, targetReplica, "rep-1"},
	}

	for _, tt := range keyTests {
		t.Run(tt.name, func(t *testing.T) {
			m := newConnectTargetModel(primary, replicas)
			updated, _ := m.Update(tt.key)
			choice := updated.(connectTargetModel).chosen
			if choice.kind != tt.wantKind {
				t.Fatalf("expected kind %v, got %v", tt.wantKind, choice.kind)
			}
			if tt.wantReplicaID != "" && (choice.replica == nil || choice.replica.ID != tt.wantReplicaID) {
				t.Errorf("expected replica %s, got %+v", tt.wantReplicaID, choice.replica)
			}
		})
	}
}

// TestBuildPsqlCommand covers the psql handoff at the helper level: the command
// path would exec a real psql.
func TestBuildPsqlCommand(t *testing.T) {
	details := func() *common.ConnectionDetails {
		return &common.ConnectionDetails{
			Host:     "testhost",
			Port:     5432,
			Database: "testdb",
			Role:     "testuser",
		}
	}
	const psqlPath = "/usr/bin/psql"
	// The password must never appear in the connection string (process lists).
	const connString = "postgresql://testuser@testhost:5432/testdb?sslmode=require"

	t.Run("connection string and flags are passed through", func(t *testing.T) {
		d := details()
		d.Password = "explicit-pw"
		flags := []string{"--single-transaction", "-c", "SELECT 1;"}
		psqlCmd := buildPsqlCommand(&config.Config{PasswordStorage: "keyring"}, d, psqlPath, flags, api.Service{}, discardCmd())

		wantArgs := []string{psqlPath, connString, "--single-transaction", "-c", "SELECT 1;"}
		if !slices.Equal(psqlCmd.Args, wantArgs) {
			t.Errorf("expected args %v, got %v", wantArgs, psqlCmd.Args)
		}
		if !slices.Contains(psqlCmd.Env, "PGPASSWORD=explicit-pw") {
			t.Errorf("expected PGPASSWORD=explicit-pw in env, got %v", psqlCmd.Env)
		}
	})

	t.Run("keyring password becomes PGPASSWORD", func(t *testing.T) {
		keyring.MockInit()
		service := api.Service{ServiceID: "svc-psql", ProjectID: "proj-psql"}
		if err := (&common.KeyringStorage{}).Save(service, "keyring-pw", "testuser"); err != nil {
			t.Fatalf("failed to save test password: %v", err)
		}

		psqlCmd := buildPsqlCommand(&config.Config{PasswordStorage: "keyring"}, details(), psqlPath, nil, service, discardCmd())
		if !slices.Contains(psqlCmd.Env, "PGPASSWORD=keyring-pw") {
			t.Errorf("expected PGPASSWORD=keyring-pw in env, got %v", psqlCmd.Env)
		}
	})

	t.Run("pgpass storage sets no PGPASSWORD", func(t *testing.T) {
		// psql reads ~/.pgpass itself, so the env var must stay unset.
		psqlCmd := buildPsqlCommand(&config.Config{PasswordStorage: "pgpass"}, details(), psqlPath, nil, api.Service{}, discardCmd())
		for _, env := range psqlCmd.Env {
			if strings.HasPrefix(env, "PGPASSWORD=") {
				t.Errorf("expected no PGPASSWORD for pgpass storage, got %s", env)
			}
		}
	})
}

// TestLaunchPsql covers the launch failure path at the helper level, with a
// psql path that cannot exist.
func TestLaunchPsql(t *testing.T) {
	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)

	details := &common.ConnectionDetails{
		Host:     "testhost",
		Port:     5432,
		Database: "testdb",
		Role:     "testuser",
	}
	err := launchPsql(&config.Config{PasswordStorage: "none"}, details, "/nonexistent/psql", []string{"--quiet"}, api.Service{}, cmd)
	if err == nil {
		t.Error("expected error for nonexistent psql path")
	}
	if stdout.String() != "" {
		t.Errorf("expected no output, got %q", stdout.String())
	}
}
