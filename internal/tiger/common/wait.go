package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

type WaitHandler interface {
	// Message returns the current status message that should be displayed next
	// to the spinner while waiting for a service to reach some state.
	Message() string

	// Check returns true if we're done waiting/polling, and false if we should
	// continue. It also returns an error, which is either immediately returned
	// from WaitForService or temporarily shown next to the spinner depending
	// on the first return value.
	Check(resp *api.GetProjectsProjectIdServicesServiceIdResponse) (bool, error)
}

type WaitForServiceArgs struct {
	Client     *api.ClientWithResponses
	ProjectID  string
	ServiceID  string
	Handler    WaitHandler
	Output     io.Writer
	Timeout    time.Duration
	TimeoutMsg string
}

func WaitForService(ctx context.Context, args WaitForServiceArgs) error {
	ctx, cancel := context.WithTimeout(ctx, args.Timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Start the spinner
	spinner := NewSpinner(args.Output, args.Handler.Message())
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
			resp, err := args.Client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, args.ProjectID, args.ServiceID)
			if err != nil {
				spinner.Update(fmt.Sprintf("Error checking service status: %s", err))
				continue
			}

			if done, err := args.Handler.Check(resp); done {
				return err
			} else if err != nil {
				spinner.Update(fmt.Sprintf("Error checking service status: %s", err))
				continue
			}

			spinner.Update(args.Handler.Message())
		}
	}
}

type StatusWaitHandler struct {
	TargetStatus string
	Service      *api.Service
}

func (h *StatusWaitHandler) Message() string {
	return fmt.Sprintf("Service status: %s", util.DerefStr(h.Service.Status))
}

func (h *StatusWaitHandler) Check(resp *api.GetProjectsProjectIdServicesServiceIdResponse) (bool, error) {
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return true, errors.New("no response body returned from API")
		}

		// Update the passed-in service's status, so it's correct when output after waiting.
		h.Service.Status = resp.JSON200.Status

		status := util.DerefStr(resp.JSON200.Status)
		switch status {
		case h.TargetStatus:
			return true, nil
		case "FAILED", "ERROR":
			return true, fmt.Errorf("service failed with status: %s", status)
		default:
			return false, nil
		}
	case 404:
		// Can happen if user deletes service while it's still provisioning
		return true, errors.New("service not found")
	case 500:
		// Assume 500s are temporary server-side issues, and that it's safe to keep polling
		return false, errors.New("internal server error")
	default:
		// Fail on unexpected status codes
		return true, fmt.Errorf("received unexpected %s while checking service status", resp.Status())
	}
}

type DeletionWaitHandler struct {
	ServiceID string
}

func (h *DeletionWaitHandler) Message() string {
	return fmt.Sprintf("Waiting for service '%s' to be deleted", h.ServiceID)
}

func (h *DeletionWaitHandler) Check(resp *api.GetProjectsProjectIdServicesServiceIdResponse) (bool, error) {
	switch resp.StatusCode() {
	case 200:
		return false, nil
	case 404:
		return true, nil
	case 500:
		// Assume 500s are temporary server-side issues, and that it's safe to keep polling
		return false, errors.New("internal server error")
	default:
		// Fail on unexpected status codes
		return true, fmt.Errorf("received unexpected %s while checking service status", resp.Status())
	}
}
