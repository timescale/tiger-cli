package common

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	if args.Service.Status == args.TargetStatus {
		return nil
	}

	// The spinner cancels this context on Ctrl+C, which the loop below reports
	// as a canceled wait.
	ctx, cancel := context.WithTimeout(ctx, args.Timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	spinner := NewSpinner(SpinnerArgs{
		Input:   args.Input,
		Output:  args.Output,
		Message: statusMessage(args.Service),
		Cancel:  cancel,
	})
	defer spinner.Stop()

	for {
		select {
		case <-ctx.Done():
			switch {
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				return ExitWithCode(ExitTimeout, fmt.Errorf("wait timeout reached after %v - %s", args.Timeout, args.TimeoutMsg))
			case errors.Is(ctx.Err(), context.Canceled):
				return fmt.Errorf("canceled waiting - %s", args.TimeoutMsg)
			default:
				return fmt.Errorf("error waiting - %s: %w", args.TimeoutMsg, ctx.Err())
			}
		case <-ticker.C:
			resp, err := args.Client.GetServiceWithResponse(ctx, args.ProjectID, args.ServiceID)
			if err != nil {
				spinner.Update(fmt.Sprintf("Error checking service status: %s", err))
				continue
			}

			switch resp.StatusCode() {
			case 200:
				if resp.JSON200 == nil {
					return errors.New("no response body returned from API")
				}
				args.Service.Status = resp.JSON200.Status
				if args.Service.Status == args.TargetStatus {
					return nil
				}
				spinner.Update(statusMessage(args.Service))
			case 404:
				// Can happen if user deletes service while it's still provisioning
				return errors.New("service not found")
			case 500:
				// Assume 500s are temporary server-side issues, and that it's safe to keep polling
				spinner.Update("Error checking service status: internal server error")
			default:
				return fmt.Errorf("received unexpected %s while checking service status", resp.Status())
			}
		}
	}
}

func statusMessage(service *api.Service) string {
	return fmt.Sprintf("Service status: %s", service.Status)
}
