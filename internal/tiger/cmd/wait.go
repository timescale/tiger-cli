package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

type waitHandler interface {
	// message returns the current status message that should be displayed next
	// to the spinner while waiting for a service to reach some state.
	message() string

	// check returns true if we're done waiting/polling, and false if we should
	// continue. It also returns an error, which is either immediately returned
	// from waitForService or temporarily shown next to the spinner depending
	// on the first return value.
	check(resp *api.GetProjectsProjectIdServicesServiceIdResponse) (bool, error)
}

type waitForServiceArgs struct {
	client     *api.ClientWithResponses
	projectID  string
	serviceID  string
	handler    waitHandler
	output     io.Writer
	timeout    time.Duration
	timeoutMsg string
}

func waitForService(ctx context.Context, args waitForServiceArgs) error {
	ctx, cancel := context.WithTimeout(ctx, args.timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Start the spinner
	spinner := NewSpinner(args.output, args.handler.message())
	defer spinner.Stop()

	for {
		select {
		case <-ctx.Done():
			switch {
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				return exitWithCode(ExitTimeout, fmt.Errorf("wait timeout reached after %v - %s", args.timeout, args.timeoutMsg))
			case errors.Is(ctx.Err(), context.Canceled):
				return fmt.Errorf("canceled waiting - %s", args.timeoutMsg)
			default:
				return fmt.Errorf("error waiting - %s: %w", args.timeoutMsg, ctx.Err())
			}
		case <-ticker.C:
			resp, err := args.client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, args.projectID, args.serviceID)
			if err != nil {
				spinner.Update(fmt.Sprintf("Error checking service status: %s", err))
				continue
			}

			if done, err := args.handler.check(resp); done {
				return err
			} else if err != nil {
				spinner.Update(fmt.Sprintf("Error checking service status: %s", err))
				continue
			}

			spinner.Update(args.handler.message())
		}
	}
}

type statusWaitHandler struct {
	targetStatus string
	service      *api.Service
}

func (h *statusWaitHandler) message() string {
	return fmt.Sprintf("Service status: %s", util.DerefStr(h.service.Status))
}

func (h *statusWaitHandler) check(resp *api.GetProjectsProjectIdServicesServiceIdResponse) (bool, error) {
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return true, errors.New("no response body returned from API")
		}

		// Update the passed-in service's status, so it's correct when output after waiting.
		h.service.Status = resp.JSON200.Status

		status := util.DerefStr(resp.JSON200.Status)
		switch status {
		case h.targetStatus:
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

type deletionWaitHandler struct {
	serviceID string
}

func (h *deletionWaitHandler) message() string {
	return fmt.Sprintf("Waiting for service '%s' to be deleted", h.serviceID)
}

func (h *deletionWaitHandler) check(resp *api.GetProjectsProjectIdServicesServiceIdResponse) (bool, error) {
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
