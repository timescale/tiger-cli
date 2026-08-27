package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestDbTestConnectionCmd(t *testing.T) {
	setupGet := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "svc-12345", sampleService())
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "test-connection", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "missing service id",
			args:    []string{"db", "test-connection"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "missing service id via ping alias",
			args:    []string{"db", "ping"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "default service id from config",
			args: []string{"db", "test-connection"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "invalid timeout duration",
			args:    []string{"db", "test-connection", "svc-12345", "--timeout", "invalid"},
			wantErr: "invalid argument \"invalid\" for \"-t, --timeout\" flag: time: invalid duration \"invalid\"",
		},
		{
			name: "network error",
			args: []string{"db", "test-connection", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "API error",
			args: []string{"db", "test-connection", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"db", "test-connection", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "test-connection", "svc-12345", "--pooled"},
			setup:   setupGet,
			wantErr: "connection pooler not available for this service",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "negative timeout",
			args:    []string{"db", "test-connection", "svc-12345", "--timeout=-5s"},
			setup:   setupGet,
			wantErr: "timeout must be positive or zero, got -5s",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// The dial is refused instantly; pgx's error text is
			// environment-dependent, hence the non-exact matches.
			name: "unreachable server",
			args: []string{"db", "test-connection", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("127.0.0.1"), Port: new(1)}
				}))
			},
			wantErr: matchFunc(func(t *testing.T, got string) {
				if !strings.Contains(got, "127.0.0.1") {
					t.Errorf("error = %q, want it to name the unreachable host", got)
				}
			}),
			wantStderr: matchPrefix("Connection failed: "),
			checks:     []checkFunc{checkExitCode(2)},
		},
		{
			// 192.0.2.0/24 (TEST-NET-1) is non-routable, so the dial hangs
			// until the --timeout deadline fires; pgx's error text is
			// environment-dependent.
			name: "connection timeout",
			args: []string{"db", "test-connection", "svc-12345", "--timeout", "250ms"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("192.0.2.1"), Port: new(5432)}
				}))
			},
			wantErr: matchFunc(func(t *testing.T, got string) {
				if !strings.Contains(got, "192.0.2.1") {
					t.Errorf("error = %q, want it to name the unreachable host", got)
				}
			}),
			wantStderr: "Connection timeout after 250ms\n",
			checks:     []checkFunc{checkExitCode(common.ExitTimeout)},
		},
	}

	runCmdTests(t, tests)
}

// TestIsConnectionRejected stays at helper level: a real 57P03 rejection needs
// a live PostgreSQL server that is starting up or out of connection slots.
func TestIsConnectionRejected(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "cannot connect now (57P03)",
			err:  &pgconn.PgError{Code: "57P03", Message: "the database system is starting up"},
			want: true,
		},
		{
			name: "authentication failed (28P01)",
			err:  &pgconn.PgError{Code: "28P01", Message: "password authentication failed for user \"test\""},
			want: false,
		},
		{
			name: "invalid authorization (28000)",
			err:  &pgconn.PgError{Code: "28000", Message: "role \"nonexistent\" does not exist"},
			want: false,
		},
		{
			name: "database does not exist (3D000)",
			err:  &pgconn.PgError{Code: "3D000", Message: "database \"nonexistent\" does not exist"},
			want: false,
		},
		{
			name: "non-postgres error",
			err:  fmt.Errorf("dial tcp: connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConnectionRejected(tt.err); got != tt.want {
				t.Errorf("isConnectionRejected(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
