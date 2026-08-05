package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
)

// A READY status means the control plane finished reconciling, which is not the
// same as Postgres having bound its port: on a fast provision the endpoint still
// refuses connections for a few seconds after the status flips. ready.go already
// documents READY as "accepting connections", so waiting on the status alone
// leaves that promise unkept, and any caller that connects immediately (rather
// than after some incidental latency) races it.
//
// WaitForConnectable closes that gap by probing the endpoint until it answers.

// DefaultConnectableTimeout bounds the endpoint probe. It is deliberately much
// shorter than the provisioning wait: once the control plane reports READY the
// endpoint comes up in seconds, so a longer wait here only delays the report
// that we could not reach it.
const DefaultConnectableTimeout = 2 * time.Minute

// connectableProbeTimeout bounds a single connection attempt, so one blackholed
// dial can't consume the whole budget.
const connectableProbeTimeout = 5 * time.Second

type ConnectableWaitArgs struct {
	Client    *api.ClientWithResponses
	ProjectID string
	ServiceID string

	// Role is the database role to probe with (e.g. "tsdbadmin").
	Role string

	// InitialPassword lets the probe authenticate fully. It is optional: without
	// it the server answers with an auth error, which still proves the endpoint
	// is serving (see classifyProbe).
	InitialPassword string

	Output io.Writer

	// Timeout bounds the whole probe. Zero means DefaultConnectableTimeout.
	Timeout time.Duration
}

// WaitForConnectable blocks until the service's Postgres endpoint answers and
// reports whether it got there. It is best-effort by design: an unverified
// endpoint warns on Output instead of failing, because a service can be
// legitimately unreachable from where the CLI runs (VPC-only, IP allowlist)
// while being perfectly healthy, and failing those users buys nothing.
func WaitForConnectable(ctx context.Context, args ConnectableWaitArgs) bool {
	timeout := args.Timeout
	if timeout <= 0 {
		timeout = DefaultConnectableTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	verified, cause := args.pollEndpoint(ctx)
	if !verified {
		warnUnverified(args.Output, cause)
	}
	return verified
}

// pollEndpoint owns the spinner so it is always stopped before the caller prints
// anything, and returns the last failure to explain why it gave up.
func (args ConnectableWaitArgs) pollEndpoint(ctx context.Context) (bool, error) {
	spinner := NewSpinner(args.Output, "Waiting for the endpoint to accept connections")
	defer spinner.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		verdict, err := args.probe(ctx)
		switch verdict {
		case probeServing:
			return true, nil
		case probeUnreachable:
			// Not "not yet" but "not from here": waiting longer won't help.
			return false, err
		}

		if err != nil {
			spinner.Update(fmt.Sprintf("Endpoint not accepting connections yet: %s", err))
		}

		select {
		case <-ctx.Done():
			return false, err
		case <-ticker.C:
		}
	}
}

func warnUnverified(out io.Writer, cause error) {
	fmt.Fprintf(out, "⚠️  Warning: service reports READY but its endpoint could not be verified from here")
	if cause != nil {
		fmt.Fprintf(out, " (%s)", cause)
	}
	fmt.Fprintf(out, ".\n    The service may still be starting, or may not be reachable from this network.\n")
}

// probe fetches the current service and makes one connection attempt. The
// service is re-fetched every round because the create response predates the
// endpoint being assigned, so the host/port can appear part-way through.
func (args ConnectableWaitArgs) probe(ctx context.Context) (probeVerdict, error) {
	resp, err := args.Client.GetServiceWithResponse(ctx, args.ProjectID, args.ServiceID)
	if err != nil {
		return probeNotYet, err
	}
	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return probeNotYet, fmt.Errorf("unexpected %s while fetching service", resp.Status())
	}

	details, err := GetConnectionDetails(*resp.JSON200, ConnectionDetailsOptions{
		Role:            args.Role,
		WithPassword:    true,
		InitialPassword: args.InitialPassword,
	})
	if err != nil {
		// Typically "endpoint host not available": not assigned yet.
		return probeNotYet, err
	}

	attemptCtx, cancel := context.WithTimeout(ctx, connectableProbeTimeout)
	defer cancel()

	conn, err := pgx.Connect(attemptCtx, details.String())
	if err != nil {
		return classifyProbe(ctx, err), err
	}
	defer conn.Close(attemptCtx)

	if err := conn.Ping(attemptCtx); err != nil {
		return classifyProbe(ctx, err), err
	}
	return probeServing, nil
}

// probeVerdict is what one connection attempt tells us about the endpoint.
type probeVerdict int

const (
	// probeNotYet: the endpoint isn't up but plausibly will be. Keep waiting.
	probeNotYet probeVerdict = iota
	// probeServing: Postgres answered. Ready, even if it refused our credentials.
	probeServing
	// probeUnreachable: we can't tell from here. Stop waiting and say so.
	probeUnreachable
)

// classifyProbe decides what a failed connection attempt means.
//
// The load-bearing rule: any Postgres protocol error proves the server is up and
// talking, so it counts as serving even when it's an auth failure. That keeps
// the probe useful without credentials, which matters because
// --password-storage=none leaves the CLI with no password to offer.
//
// parent is the overall wait context, used to tell "this attempt timed out" from
// "the whole budget ran out".
func classifyProbe(parent context.Context, err error) probeVerdict {
	if err == nil {
		return probeServing
	}

	// 57P03 (CANNOT_CONNECT_NOW) is the one protocol error that means "up but
	// still starting", so it's the one that keeps us waiting.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "57P03" {
			return probeNotYet
		}
		return probeServing
	}

	switch {
	// A single slow attempt is inconclusive; keep waiting unless the whole
	// budget is spent, in which case the caller reports it unverified.
	case errors.Is(err, context.DeadlineExceeded):
		if parent.Err() != nil {
			return probeUnreachable
		}
		return probeNotYet
	case errors.Is(err, context.Canceled):
		return probeUnreachable
	// Nothing listening yet: the exact shape of the race this fixes.
	case errors.Is(err, syscall.ECONNREFUSED):
		return probeNotYet
	// Endpoint DNS can lag the status flip.
	case isDNSNotFound(err):
		return probeNotYet
	// Any other dial-stage failure. Checked after the specific cases above so
	// the common ones stay self-documenting, and so the fix doesn't depend on
	// one errno matching identically on every platform we release for.
	case isDialFailure(err):
		return probeNotYet
	}

	// Anything else (TLS failure, unexpected protocol framing) is not something
	// more waiting resolves.
	return probeUnreachable
}

func isDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func isDialFailure(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr)
}
