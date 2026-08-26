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
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
			check:   checkExitCode(common.ExitInvalidParameters),
		},
		{
			name:    "missing service id",
			args:    []string{"db", "test-connection"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
			check:   checkExitCode(common.ExitInvalidParameters),
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
			check:   checkExitCode(common.ExitInvalidParameters),
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
			check:   checkExitCode(common.ExitInvalidParameters),
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
			check:   checkExitCode(common.ExitInvalidParameters),
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
			check:   checkExitCode(common.ExitInvalidParameters),
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "test-connection", "svc-12345", "--pooled"},
			setup:   setupGet,
			wantErr: "connection pooler not available for this service",
			check:   checkExitCode(common.ExitInvalidParameters),
		},
		{
			name:    "negative timeout",
			args:    []string{"db", "test-connection", "svc-12345", "--timeout=-5s"},
			setup:   setupGet,
			wantErr: "timeout must be positive or zero, got -5s",
			check:   checkExitCode(common.ExitInvalidParameters),
		},
	}

	runCmdTests(t, tests)

	// Actual connection attempts produce environment-dependent pgx error text,
	// so these assert the exit code and stderr shape instead of exact output.
	t.Run("unreachable server", func(t *testing.T) {
		result := runCommand(t, []string{"db", "test-connection", "svc-12345"},
			func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("127.0.0.1"), Port: new(1)}
				}))
			})
		checkExitCode(2)(t, result)
		if !strings.HasPrefix(result.stderr, "Connection failed: ") {
			t.Errorf("expected stderr to start with %q, got %q", "Connection failed: ", result.stderr)
		}
	})

	t.Run("connection timeout", func(t *testing.T) {
		// 192.0.2.0/24 (TEST-NET-1) is non-routable, so the dial hangs until
		// the --timeout deadline fires.
		result := runCommand(t, []string{"db", "test-connection", "svc-12345", "--timeout", "250ms"},
			func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("192.0.2.1"), Port: new(5432)}
				}))
			})
		checkExitCode(common.ExitTimeout)(t, result)
		assertOutput(t, result.stderr, "Connection timeout after 250ms\n")
	})
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
