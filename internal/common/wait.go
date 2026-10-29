package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
)

type WaitForServiceArgs struct {
	Client    api.ClientWithResponsesInterface
	ProjectID string
	ServiceID string

	// Service is the service as last returned by the API. Its status decides
	// whether polling is needed at all, and it is updated in place as polling
	// proceeds so the caller can output the final state afterwards.
	Service *api.Service

	// TargetStatus ends the wait once the service reports it. Every other
	// status, including UNSTABLE, keeps polling: transitional states pass and
	// unstable services often recover on their own.
	TargetStatus api.DeployStatus

	// Input lets the spinner pick up a Ctrl+C and cancel the wait. See
	// [SpinnerArgs.Input] for why the spinner needs stdin at all.
	Input      io.Reader
	Output     io.Writer
	Timeout    time.Duration
	TimeoutMsg string
}

// WaitForService polls a service until it reports TargetStatus, showing its
// current status next to a spinner in the meantime.
func WaitForService(ctx context.Context, args WaitForServiceArgs) error {
	return waitForStatus(ctx, waitArgs{
		kind:         "service",
		title:        "Service",
		status:       &args.Service.Status,
		targetStatus: args.TargetStatus,
		input:        args.Input,
		output:       args.Output,
		timeout:      args.Timeout,
		timeoutMsg:   args.TimeoutMsg,
		poll: func(ctx context.Context) (int, string, *api.DeployStatus, error) {
			resp, err := args.Client.GetServiceWithResponse(ctx, args.ProjectID, args.ServiceID)
			if err != nil {
				return 0, "", nil, err
			}
			if resp.JSON200 == nil {
				return resp.StatusCode(), resp.Status(), nil, nil
			}
			return resp.StatusCode(), resp.Status(), &resp.JSON200.Status, nil
		},
	})
}

type WaitForBranchArgs struct {
	Client    api.ClientWithResponsesInterface
	ProjectID string
	ServiceID string // the branch's parent
	BranchID  string

	// Branch works as WaitForServiceArgs.Service: its status decides whether
	// polling is needed at all, and is updated in place as polling proceeds.
	Branch *api.Branch

	// TargetStatus, Input, Output, Timeout, and TimeoutMsg work as in
	// WaitForServiceArgs.
	TargetStatus api.DeployStatus
	Input        io.Reader
	Output       io.Writer
	Timeout      time.Duration
	TimeoutMsg   string
}

// WaitForBranch polls a branch until it reports TargetStatus, showing its
// current status next to a spinner in the meantime.
func WaitForBranch(ctx context.Context, args WaitForBranchArgs) error {
	return waitForStatus(ctx, waitArgs{
		kind:         "branch",
		title:        "Branch",
		status:       &args.Branch.Status,
		targetStatus: args.TargetStatus,
		input:        args.Input,
		output:       args.Output,
		timeout:      args.Timeout,
		timeoutMsg:   args.TimeoutMsg,
		poll: func(ctx context.Context) (int, string, *api.DeployStatus, error) {
			resp, err := args.Client.GetBranchWithResponse(ctx, args.ProjectID, args.ServiceID, args.BranchID)
			if err != nil {
				return 0, "", nil, err
			}
			if resp.JSON200 == nil {
				return resp.StatusCode(), resp.Status(), nil, nil
			}
			return resp.StatusCode(), resp.Status(), &resp.JSON200.Status, nil
		},
	})
}

// waitArgs is what waitForStatus needs to poll any resource with a
// DeployStatus.
type waitArgs struct {
	// kind names the resource in error messages, and title in the spinner's
	// status message.
	kind  string
	title string

	// status is updated in place as polling proceeds.
	status       *api.DeployStatus
	targetStatus api.DeployStatus

	// poll fetches the resource once, returning the HTTP status code and
	// text, and the resource's status when the response carried a body.
	poll func(ctx context.Context) (code int, httpStatus string, current *api.DeployStatus, err error)

	input      io.Reader
	output     io.Writer
	timeout    time.Duration
	timeoutMsg string
}

// waitForStatus is the poll loop behind WaitForService and WaitForBranch.
func waitForStatus(ctx context.Context, args waitArgs) error {
	if *args.status == args.targetStatus {
		return nil
	}

	// The spinner cancels this context on Ctrl+C, which the loop below reports
	// as a canceled wait.
	ctx, cancel := context.WithTimeout(ctx, args.timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	spinner := NewSpinner(SpinnerArgs{
		Input:   args.input,
		Output:  args.output,
		Message: statusMessage(args.title, *args.status),
		Cancel:  cancel,
	})
	defer spinner.Stop()

	for {
		select {
		case <-ctx.Done():
			switch {
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				return ExitWithCode(ExitTimeout, fmt.Errorf("wait timeout reached after %v - %s", args.timeout, args.timeoutMsg))
			case errors.Is(ctx.Err(), context.Canceled):
				return fmt.Errorf("canceled waiting - %s", args.timeoutMsg)
			default:
				return fmt.Errorf("error waiting - %s: %w", args.timeoutMsg, ctx.Err())
			}
		case <-ticker.C:
			code, httpStatus, current, err := args.poll(ctx)
			if err != nil {
				spinner.Update(fmt.Sprintf("Error checking %s status: %s", args.kind, err))
				continue
			}

			switch code {
			case http.StatusOK:
				if current == nil {
					return errors.New("no response body returned from API")
				}
				*args.status = *current
				if *args.status == args.targetStatus {
					return nil
				}
				spinner.Update(statusMessage(args.title, *args.status))
			case http.StatusNotFound:
				// Can happen if user deletes the resource while it's still provisioning
				return fmt.Errorf("%s not found", args.kind)
			case http.StatusInternalServerError:
				// Assume 500s are temporary server-side issues, and that it's safe to keep polling
				spinner.Update(fmt.Sprintf("Error checking %s status: internal server error", args.kind))
			default:
				return fmt.Errorf("received unexpected %s while checking %s status", httpStatus, args.kind)
			}
		}
	}
}

func statusMessage(title string, status api.DeployStatus) string {
	return fmt.Sprintf("%s status: %s", title, status)
}
