package cmd

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

const passwordNormalizedMsg = "password contains characters that database clients handle inconsistently, " +
	"such as ligatures, full-width letters, non-ASCII spaces, or invisible characters. Choose a different password"

func TestDbCreateRoleCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:    "missing name flag",
			args:    []string{"db", "create", "role", "svc-12345"},
			wantErr: "required flag(s) \"name\" not set",
		},
		{
			name:    "empty name flag",
			args:    []string{"db", "create", "role", "svc-12345", "--name", ""},
			wantErr: "--name is required",
		},
		{
			name:    "empty name flag via user alias",
			args:    []string{"db", "create", "user", "svc-12345", "--name", ""},
			wantErr: "--name is required",
		},
		{
			name:    "not logged in",
			args:    []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "missing service id",
			args:    []string{"db", "create", "role", "--name", "ai_analyst"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			// The blanket case is refused before the ref is resolved, so no
			// call is registered here.
			name:    "read-only all refuses",
			args:    []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "read-only prod refuses PROD service",
			args:    []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedTarget("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			// The missing endpoint stops the command right after the gate,
			// proving prod let a DEV service through.
			name: "read-only prod allows DEV service",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				tag := "DEV"
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Metadata = &api.ServiceMetadata{Environment: &tag}
					s.Endpoint = nil
				}))
			},
			wantErr: "failed to build connection string: endpoint not available",
		},
		{
			// The missing endpoint stops the command before any connection
			// attempt, proving the config default reached the service lookup.
			name: "default service id from config",
			args: []string{"db", "create", "role", "--name", "ai_analyst"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = nil
				}))
			},
			wantErr: "failed to build connection string: endpoint not available",
		},
		{
			name: "network error",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetError(m, "svc-12345", errors.New("connection refused"))
			},
			wantErr: `failed to resolve service or read replica set 'svc-12345': connection refused`,
		},
		{
			name: "service not found",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetNotFound(m, "svc-12345")
			},
			wantErr: "service or read replica set 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "ambiguous name refused",
			args: []string{"db", "create", "role", "my-api-db", "--name", "ai_analyst"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "my-api-db", http.StatusBadRequest, &api.ClientError{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "svc-12345", http.StatusOK, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "read replica rejected",
			args: []string{"db", "create", "role", "rep-67890", "--name", "ai_analyst"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "rep-67890", sampleReplica())
				expectGetService(m, "svc-12345", sampleService())
			},
			wantErr: "'rep-67890' is a read replica; create the role on its primary service 'svc-12345' instead",
		},
		{
			// With no --password or TIGER_NEW_PASSWORD, the password is
			// generated before connecting.
			name: "password generation fails",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			opts: []runOption{withGenerateSecurePassword(32, "", errors.New("entropy unavailable"))},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService())
			},
			wantErr: "failed to determine password: entropy unavailable",
		},
		{
			// NFKC folds the ligature into "fi".
			name: "password with ligature refused",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst", "--password", "\ufb01sh-password"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService())
			},
			wantErr: passwordNormalizedMsg,
		},
		{
			// NFKC composes e and the combining accent into one character.
			name: "password with decomposed accent refused",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst", "--password", "pe\u0301ncil-password"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService())
			},
			wantErr: passwordNormalizedMsg,
		},
		{
			// SASLprep replaces non-ASCII spaces with an ASCII space.
			name: "password with no-break space refused",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst", "--password", "my\u00a0password"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService())
			},
			wantErr: passwordNormalizedMsg,
		},
		{
			// SASLprep removes the variation selector that follows the heart.
			name: "password from TIGER_NEW_PASSWORD with emoji variation selector refused",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			opts: []runOption{withEnv("TIGER_NEW_PASSWORD", "my-\u2764\ufe0f-password")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService())
			},
			wantErr: passwordNormalizedMsg,
		},
		{
			// The missing endpoint stops the command right after the check,
			// proving a password SASLprep leaves unchanged gets through,
			// single quotes included.
			name: "non-ASCII password accepted",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst", "--password", "p\u00e4ssw\u00f6rd-'\u5bc6\u7801'-\U0001F980"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = nil
				}))
			},
			wantErr: "failed to build connection string: endpoint not available",
		},
		{
			name: "endpoint not available",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = nil
				}))
			},
			wantErr: "failed to build connection string: endpoint not available",
		},
		{
			// Branches are a preview, so the help mentions them only behind
			// the gate. The generated docs/cli reference covers the ungated
			// help.
			name:       "help mentions branches when experimental",
			args:       []string{"db", "create", "role", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("You can also pass a branch, by ID or name, to create the role on that branch."),
		},
	})
}

// The SQL builders below can't be exercised through the command without a live
// database (the statements only run after a successful pgx connection), so they
// keep helper-level tests.

func TestBuildCreateRoleSQL(t *testing.T) {
	const v = "SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy"
	tests := []struct {
		name      string
		roleName  string
		verifier  string
		fromRoles []string
		want      string
	}{
		{
			name:     "no from roles",
			roleName: "test_role",
			verifier: v,
			want:     `CREATE ROLE "test_role" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy'`,
		},
		{
			name:      "empty from roles",
			roleName:  "test_role",
			verifier:  v,
			fromRoles: []string{},
			want:      `CREATE ROLE "test_role" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy'`,
		},
		{
			name:      "single from role",
			roleName:  "ai_analyst",
			verifier:  v,
			fromRoles: []string{"app_role"},
			want:      `CREATE ROLE "ai_analyst" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy' IN ROLE "app_role"`,
		},
		{
			name:      "multiple from roles",
			roleName:  "ai_analyst",
			verifier:  v,
			fromRoles: []string{"app_role", "readonly_role", "reporting_role"},
			want:      `CREATE ROLE "ai_analyst" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy' IN ROLE "app_role", "readonly_role", "reporting_role"`,
		},
		{
			name:     "case preserved by quoting",
			roleName: "MixedCase_Role",
			verifier: v,
			want:     `CREATE ROLE "MixedCase_Role" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy'`,
		},
		{
			name:     "role name with spaces",
			roleName: "role with spaces",
			verifier: v,
			want:     `CREATE ROLE "role with spaces" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy'`,
		},
		{
			name:     "role name with dashes",
			roleName: "role-with-dashes",
			verifier: v,
			want:     `CREATE ROLE "role-with-dashes" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy'`,
		},
		{
			// pgx.Identifier.Sanitize doubles embedded quotes, keeping the
			// injection attempt inside the quoted identifier.
			name:     "injection in role name",
			roleName: `test"; DROP TABLE users; --`,
			verifier: v,
			want:     `CREATE ROLE "test""; DROP TABLE users; --" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy'`,
		},
		{
			name:      "injection in from role",
			roleName:  "safe_role",
			verifier:  v,
			fromRoles: []string{`admin"; DROP TABLE users; --`},
			want:      `CREATE ROLE "safe_role" WITH LOGIN PASSWORD 'SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:c2VydmVy' IN ROLE "admin""; DROP TABLE users; --"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOutput(t, buildCreateRoleSQL(tt.roleName, tt.verifier, tt.fromRoles), tt.want)
		})
	}
}

func TestBuildReadOnlyAlterSQL(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		want     string
	}{
		{
			name:     "simple role name",
			roleName: "ai_analyst",
			want:     `ALTER ROLE "ai_analyst" SET tsdb_admin.read_only_role = true`,
		},
		{
			name:     "injection in role name",
			roleName: `test"; DROP TABLE users; --`,
			want:     `ALTER ROLE "test""; DROP TABLE users; --" SET tsdb_admin.read_only_role = true`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOutput(t, buildReadOnlyAlterSQL(tt.roleName), tt.want)
		})
	}
}

func TestBuildStatementTimeoutAlterSQL(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		timeout  time.Duration
		want     string
	}{
		{
			name:     "30 seconds",
			roleName: "test_role",
			timeout:  30 * time.Second,
			want:     `ALTER ROLE "test_role" SET statement_timeout = 30000`,
		},
		{
			name:     "5 minutes",
			roleName: "test_role",
			timeout:  5 * time.Minute,
			want:     `ALTER ROLE "test_role" SET statement_timeout = 300000`,
		},
		{
			name:     "1 hour",
			roleName: "test_role",
			timeout:  time.Hour,
			want:     `ALTER ROLE "test_role" SET statement_timeout = 3600000`,
		},
		{
			name:     "sub-second precision in milliseconds",
			roleName: "test_role",
			timeout:  1500 * time.Millisecond,
			want:     `ALTER ROLE "test_role" SET statement_timeout = 1500`,
		},
		{
			name:     "injection in role name",
			roleName: `test"; DROP TABLE users; --`,
			timeout:  30 * time.Second,
			want:     `ALTER ROLE "test""; DROP TABLE users; --" SET statement_timeout = 30000`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOutput(t, buildStatementTimeoutAlterSQL(tt.roleName, tt.timeout), tt.want)
		})
	}
}

// The verifier's salt is random, and whether Postgres accepts it only shows on
// a live database (integration_test.go logs in with the new role), so the
// derivation keeps a helper-level test against a fixed salt. The expected
// verifiers were derived independently, with Python's hashlib.
func TestBuildSCRAMSHA256Verifier(t *testing.T) {
	// The salt from the SCRAM-SHA-256 example in RFC 7677, section 3.
	salt, err := base64.StdEncoding.DecodeString("W22ZaJ0SNY7soEsUEjb6gQ==")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		password string
		want     string
	}{
		{
			// The RFC's example login, whose published proof and server
			// signature this verifier's keys reproduce.
			name:     "RFC 7677 example",
			password: "pencil",
			want:     "SCRAM-SHA-256$4096:W22ZaJ0SNY7soEsUEjb6gQ==$WG5d8oPm3OtcPnkdi4Uo7BkeZkBFzpcXkuLmtbsT4qY=:wfPLwcE6nTWhTAmQ7tl2KeoiWGPlZqQxSrmfPwDl2dU=",
		},
		{
			name:     "non-ASCII password hashed as UTF-8",
			password: "p\u00e4ssw\u00f6rd",
			want:     "SCRAM-SHA-256$4096:W22ZaJ0SNY7soEsUEjb6gQ==$dcgqTWLkt/QY/G2TTG2Kx054l2TY/d1/rrqpxFf42c8=:1J1wEQIBJAVfD0SDivXshqbZYR5KFg/C5ltFBHBSzbc=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSCRAMSHA256Verifier(tt.password, salt, 4096)
			if err != nil {
				t.Fatal(err)
			}
			assertOutput(t, got, tt.want)
		})
	}
}
