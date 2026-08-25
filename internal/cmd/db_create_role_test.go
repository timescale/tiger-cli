package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestDbCreateRoleCmd(t *testing.T) {
	tests := []cmdTest{
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
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
		},
		{
			name:    "missing service id",
			args:    []string{"db", "create", "role", "--name", "ai_analyst"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
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
			name: "read replica rejected",
			args: []string{"db", "create", "role", "rep-67890", "--name", "ai_analyst"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica())
			},
			wantErr: "\"rep-67890\" is a read replica; create the role on its primary service \"svc-12345\" instead",
		},
		{
			name: "endpoint not available",
			args: []string{"db", "create", "role", "svc-12345", "--name", "ai_analyst"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = nil
				}))
			},
			wantErr: "failed to build connection string: service endpoint not available",
		},
	}

	runCmdTests(t, tests)
}

// The SQL builders below can't be exercised through the command without a live
// database (the statements only run after a successful pgx connection), so they
// keep helper-level tests.

func TestBuildCreateRoleSQL(t *testing.T) {
	tests := []struct {
		name      string
		roleName  string
		password  string
		fromRoles []string
		want      string
	}{
		{
			name:     "no from roles",
			roleName: "test_role",
			password: "'my_password'",
			want:     `CREATE ROLE "test_role" WITH LOGIN PASSWORD 'my_password'`,
		},
		{
			name:      "empty from roles",
			roleName:  "test_role",
			password:  "'test_pass'",
			fromRoles: []string{},
			want:      `CREATE ROLE "test_role" WITH LOGIN PASSWORD 'test_pass'`,
		},
		{
			name:      "single from role",
			roleName:  "ai_analyst",
			password:  "'test_pass'",
			fromRoles: []string{"app_role"},
			want:      `CREATE ROLE "ai_analyst" WITH LOGIN PASSWORD 'test_pass' IN ROLE "app_role"`,
		},
		{
			name:      "multiple from roles",
			roleName:  "ai_analyst",
			password:  "'test_pass'",
			fromRoles: []string{"app_role", "readonly_role", "reporting_role"},
			want:      `CREATE ROLE "ai_analyst" WITH LOGIN PASSWORD 'test_pass' IN ROLE "app_role", "readonly_role", "reporting_role"`,
		},
		{
			name:     "case preserved by quoting",
			roleName: "MixedCase_Role",
			password: "'test_pass'",
			want:     `CREATE ROLE "MixedCase_Role" WITH LOGIN PASSWORD 'test_pass'`,
		},
		{
			name:     "role name with spaces",
			roleName: "role with spaces",
			password: "'test_pass'",
			want:     `CREATE ROLE "role with spaces" WITH LOGIN PASSWORD 'test_pass'`,
		},
		{
			name:     "role name with dashes",
			roleName: "role-with-dashes",
			password: "'test_pass'",
			want:     `CREATE ROLE "role-with-dashes" WITH LOGIN PASSWORD 'test_pass'`,
		},
		{
			// pgx.Identifier.Sanitize doubles embedded quotes, keeping the
			// injection attempt inside the quoted identifier.
			name:     "injection in role name",
			roleName: `test"; DROP TABLE users; --`,
			password: "'test_pass'",
			want:     `CREATE ROLE "test""; DROP TABLE users; --" WITH LOGIN PASSWORD 'test_pass'`,
		},
		{
			name:      "injection in from role",
			roleName:  "safe_role",
			password:  "'test_pass'",
			fromRoles: []string{`admin"; DROP TABLE users; --`},
			want:      `CREATE ROLE "safe_role" WITH LOGIN PASSWORD 'test_pass' IN ROLE "admin""; DROP TABLE users; --"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOutput(t, buildCreateRoleSQL(tt.roleName, tt.password, tt.fromRoles), tt.want)
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
