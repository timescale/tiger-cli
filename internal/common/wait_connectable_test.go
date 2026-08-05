package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/util"
)

// liveCtx is a context that hasn't expired, i.e. budget remaining.
func liveCtx() context.Context { return context.Background() }

// spentCtx is a context whose deadline has already passed, i.e. budget gone.
func spentCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	return ctx
}

func TestClassifyProbe(t *testing.T) {
	tests := []struct {
		name   string
		parent context.Context
		err    error
		want   probeVerdict
	}{
		{
			name: "success",
			err:  nil,
			want: probeServing,
		},
		{
			// The race this exists to fix: control plane said READY, nothing listening.
			name: "connection refused",
			err:  &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
			want: probeNotYet,
		},
		{
			name: "endpoint dns not resolving yet",
			err:  &net.OpError{Op: "dial", Err: &net.DNSError{IsNotFound: true}},
			want: probeNotYet,
		},
		{
			// 57P03 is the server saying "up, still starting".
			name: "cannot connect now",
			err:  &pgconn.PgError{Code: "57P03"},
			want: probeNotYet,
		},
		{
			// The load-bearing case: no password to offer, server rejects us, but
			// it answered, so the endpoint is serving.
			name: "invalid password proves the server is serving",
			err:  &pgconn.PgError{Code: "28P01"},
			want: probeServing,
		},
		{
			name: "insufficient privilege also proves it is serving",
			err:  &pgconn.PgError{Code: "42501"},
			want: probeServing,
		},
		{
			name: "wrapped pg error is still unwrapped",
			err:  fmt.Errorf("failed to connect: %w", &pgconn.PgError{Code: "28P01"}),
			want: probeServing,
		},
		{
			name:   "attempt timed out but budget remains",
			parent: liveCtx(),
			err:    context.DeadlineExceeded,
			want:   probeNotYet,
		},
		{
			name: "attempt timed out and budget is gone",
			// parent set to a spent context below.
			err:  context.DeadlineExceeded,
			want: probeUnreachable,
		},
		{
			name: "canceled",
			err:  context.Canceled,
			want: probeUnreachable,
		},
		{
			// Waiting doesn't fix TLS. Report rather than spin.
			name: "tls failure is not a waiting problem",
			err:  errors.New("tls: failed to verify certificate"),
			want: probeUnreachable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := tt.parent
			if parent == nil {
				if tt.want == probeUnreachable && errors.Is(tt.err, context.DeadlineExceeded) {
					parent = spentCtx(t)
				} else {
					parent = liveCtx()
				}
			}
			if got := classifyProbe(parent, tt.err); got != tt.want {
				t.Errorf("classifyProbe(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// A refused dial must keep the probe waiting regardless of which platform's
// errno shape it arrives in, since that is the failure the fix targets.
func TestClassifyProbeRefusedVariants(t *testing.T) {
	for _, err := range []error{
		syscall.ECONNREFUSED,
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
		fmt.Errorf("dial tcp 10.0.0.1:5432: %w", syscall.ECONNREFUSED),
		&net.OpError{Op: "dial", Err: errors.New("some platform-specific refusal")},
	} {
		if got := classifyProbe(liveCtx(), err); got != probeNotYet {
			t.Errorf("classifyProbe(%v) = %v, want probeNotYet", err, got)
		}
	}
}

// closedPort returns a port with nothing listening on it, so a dial gets
// refused: the same signal the endpoint gives while Postgres is still starting.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}
	return port
}

// A refused endpoint must give up on its own budget and warn, never hang: this
// runs inside `tiger service create`, so an unbounded loop would wedge the CLI.
func TestWaitForConnectableGivesUpWithinBudget(t *testing.T) {
	port := closedPort(t)
	host := "127.0.0.1"
	status := api.READY

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.Service{
			ServiceId: util.Ptr("svc-test"),
			Status:    &status,
			Endpoint:  &api.Endpoint{Host: &host, Port: &port},
		}); err != nil {
			t.Errorf("encoding service: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client, err := api.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	var out strings.Builder
	budget := 2 * time.Second
	start := time.Now()

	// InitialPassword is set so the probe skips password storage entirely; this
	// test is about the loop, not about credential lookup.
	verified := WaitForConnectable(context.Background(), ConnectableWaitArgs{
		Client:          client,
		ProjectID:       "proj-test",
		ServiceID:       "svc-test",
		Role:            "tsdbadmin",
		InitialPassword: "irrelevant",
		Output:          &out,
		Timeout:         budget,
	})
	elapsed := time.Since(start)

	if verified {
		t.Error("WaitForConnectable() = true for a refused endpoint, want false")
	}
	if elapsed > budget+5*time.Second {
		t.Errorf("took %v, want it to respect its %v budget", elapsed, budget)
	}
	if !strings.Contains(out.String(), "could not be verified") {
		t.Errorf("output = %q, want an unverified warning", out.String())
	}
}

func TestWarnUnverifiedMentionsCause(t *testing.T) {
	var sb strings.Builder
	warnUnverified(&sb, errors.New("connection refused"))
	got := sb.String()

	for _, want := range []string{"Warning", "READY", "connection refused"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnUnverified() = %q, want it to contain %q", got, want)
		}
	}
}

func TestWarnUnverifiedWithoutCause(t *testing.T) {
	var sb strings.Builder
	warnUnverified(&sb, nil)
	if got := sb.String(); !strings.Contains(got, "Warning") || strings.Contains(got, "()") {
		t.Errorf("warnUnverified(nil) = %q, want a warning with no empty parens", got)
	}
}
