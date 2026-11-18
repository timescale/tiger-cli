package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestDbPingCmd(t *testing.T) {
	setupGet := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveTarget(m, "svc-12345", sampleService())
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "ping", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "missing service id",
			args:    []string{"db", "ping"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "missing service id via test alias",
			args:    []string{"db", "test"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "missing service id via test-connection alias",
			args:    []string{"db", "test-connection"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "default service id from config",
			args: []string{"db", "ping"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetError(m, "svc-12345", errors.New("connection refused"))
			},
			wantErr: `failed to resolve service or read replica set 'svc-12345': connection refused`,
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "invalid timeout duration",
			args:    []string{"db", "ping", "svc-12345", "--timeout", "invalid"},
			wantErr: "invalid argument \"invalid\" for \"-t, --timeout\" flag: must be a duration like \"30s\" or \"5m\"",
		},
		{
			name: "network error",
			args: []string{"db", "ping", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetError(m, "svc-12345", errors.New("connection refused"))
			},
			wantErr: `failed to resolve service or read replica set 'svc-12345': connection refused`,
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "service not found",
			args: []string{"db", "ping", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetNotFound(m, "svc-12345")
			},
			wantErr: "service or read replica set 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "ambiguous name refused",
			args: []string{"db", "ping", "my-api-db"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "my-api-db", http.StatusBadRequest, &api.ClientError{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"db", "ping", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "svc-12345", http.StatusOK, nil)
			},
			wantErr: "empty response from API",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "ping", "svc-12345", "--pooled"},
			mock:    setupGet,
			wantErr: "connection pooler not available for this service",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "negative timeout",
			args:    []string{"db", "ping", "svc-12345", "--timeout=-5s"},
			mock:    setupGet,
			wantErr: "timeout must be positive or zero, got -5s",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// The dial is refused instantly; pgx's error text is
			// environment-dependent, hence the non-exact matches.
			name: "unreachable server",
			args: []string{"db", "ping", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("127.0.0.1"), Port: new(1)}
				}))
			},
			wantErr: matchFunc(func(t *testing.T, got string) {
				if !strings.Contains(got, "127.0.0.1") {
					t.Errorf("error = %q, want it to name the unreachable host", got)
				}
			}),
			checks: []checkFunc{checkExitCode(2)},
		},
		{
			// 192.0.2.0/24 (TEST-NET-1) is non-routable, so the dial hangs
			// until the --timeout deadline fires; pgx's error text is
			// environment-dependent.
			name: "connection timeout",
			args: []string{"db", "ping", "svc-12345", "--timeout", "250ms"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("192.0.2.1"), Port: new(5432)}
				}))
			},
			wantErr: matchFunc(func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "connection timed out: ") || !strings.Contains(got, "192.0.2.1") {
					t.Errorf("error = %q, want a timeout naming the unreachable host", got)
				}
			}),
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			// PGCONNECT_TIMEOUT=0 removes the per-address limit, so only
			// --timeout ends the dial; pgx's error text is
			// environment-dependent.
			name: "--timeout bounds the check with no per-address limit",
			args: []string{"db", "ping", "svc-12345", "--timeout", "250ms"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{Host: new("192.0.2.1"), Port: new(5432)}
				}))
			},
			opts: []runOption{withEnv(common.ConnectTimeoutEnvVar, "0")},
			wantErr: matchFunc(func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "connection timed out: ") || !strings.Contains(got, "192.0.2.1") {
					t.Errorf("error = %q, want a timeout naming the unreachable host", got)
				}
			}),
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			// Branches are a preview, so the help mentions them only behind
			// the gate. The generated docs/cli reference covers the ungated
			// help.
			name:       "help mentions branches when experimental",
			args:       []string{"db", "ping", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("You can also pass a branch, by ID or name, to test connectivity to that branch."),
		},
	})
}

// TestConnectionFailure stays at helper level: the errors pgx joins across a
// host's addresses only arise with several servers failing in different ways
// at once, so they're built here with errors.Join, as pgx builds them.
func TestConnectionFailure(t *testing.T) {
	timeout := fmt.Errorf("192.0.2.1:5432: %w", context.DeadlineExceeded)
	starting := &pgconn.PgError{Severity: "FATAL", Code: "57P03", Message: "the database system is starting up"}
	badPassword := &pgconn.PgError{Severity: "FATAL", Code: "28P01", Message: "password authentication failed for user \"tsdbadmin\""}

	tests := []struct {
		name         string
		err          error
		wantMessage  string
		wantExitCode int
	}{
		{
			name:         "rejection",
			err:          fmt.Errorf("failed to connect: %w", starting),
			wantMessage:  "server is rejecting connections: failed to connect: FATAL: the database system is starting up (SQLSTATE 57P03)",
			wantExitCode: common.ExitGeneralError,
		},
		{
			name:         "rejection beats a timeout at another address",
			err:          errors.Join(timeout, starting),
			wantMessage:  "server is rejecting connections: 192.0.2.1:5432: context deadline exceeded\nFATAL: the database system is starting up (SQLSTATE 57P03)",
			wantExitCode: common.ExitGeneralError,
		},
		{
			// Not a timeout, though one address did time out.
			name:         "other server reply beats a timeout at another address",
			err:          errors.Join(timeout, badPassword),
			wantMessage:  "192.0.2.1:5432: context deadline exceeded\nFATAL: password authentication failed for user \"tsdbadmin\" (SQLSTATE 28P01)",
			wantExitCode: 2,
		},
		{
			name:         "timeouts only",
			err:          errors.Join(timeout, fmt.Errorf("192.0.2.2:5432: %w", context.DeadlineExceeded)),
			wantMessage:  "connection timed out: 192.0.2.1:5432: context deadline exceeded\n192.0.2.2:5432: context deadline exceeded",
			wantExitCode: common.ExitTimeout,
		},
		{
			name:         "no response",
			err:          errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
			wantMessage:  "dial tcp 127.0.0.1:1: connect: connection refused",
			wantExitCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := connectionFailure(tt.err)
			var exitErr common.ExitCodeError
			if !errors.As(err, &exitErr) {
				t.Fatalf("connectionFailure() = %v, want an exit code error", err)
			}
			if err.Error() != tt.wantMessage || exitErr.ExitCode() != tt.wantExitCode {
				t.Errorf("connectionFailure() = (%q, exit %d), want (%q, exit %d)", err.Error(), exitErr.ExitCode(), tt.wantMessage, tt.wantExitCode)
			}
		})
	}
}

// TestIsConnectionTimeout stays at helper level: which error shape a deadline
// surfaces as varies by platform and pgx version (the command-level
// "connection timeout" case sees only this machine's), so the shapes are
// covered synthetically.
func TestIsConnectionTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "context deadline exceeded",
			err:  fmt.Errorf("failed to connect: %w", context.DeadlineExceeded),
			want: true,
		},
		{
			// How the dialer surfaces the deadline on Linux: an i/o timeout
			// net error, without context.DeadlineExceeded in the chain.
			name: "network timeout",
			err:  fmt.Errorf("failed to connect: %w", &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}),
			want: true,
		},
		{
			// How a DNS lookup that ran out of time surfaces, inside pgx's
			// "hostname resolving error".
			name: "DNS lookup timeout",
			err:  fmt.Errorf("hostname resolving error: %w", &net.DNSError{Err: "i/o timeout", Name: "host.example.com", IsTimeout: true}),
			want: true,
		},
		{
			name: "non-timeout network error",
			err:  fmt.Errorf("failed to connect: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")}),
			want: false,
		},
		{
			name: "unrelated error",
			err:  errors.New("something else"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConnectionTimeout(tt.err); got != tt.want {
				t.Errorf("isConnectionTimeout(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
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
			// pgx wraps a connect failure in a *pgconn.ConnectError, whose
			// fields can't be set from outside pgconn; %w wraps it the same
			// way as far as errors.As is concerned.
			name: "wrapped cannot connect now (57P03)",
			err:  fmt.Errorf("failed to connect: %w", &pgconn.PgError{Code: "57P03", Message: "the database system is starting up"}),
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
