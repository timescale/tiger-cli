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

	// InitialCheck returns true if we don't need to begin the waiting/polling
	// process, and false if we should.  It also returns an error, which is
	// either immediately returned from WaitForService or temporarily shown
	// next to the spinner depending on the first return value.
	InitialCheck() (bool, error)

	// Check returns true if we're done waiting/polling, and false if we should
	// continue. It also returns an error, which is either immediately returned
	// from WaitForService or temporarily shown next to the spinner depending
	// on the first return value.
	Check(resp *api.GetServiceResponse) (bool, error)
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

// pollStep performs one poll iteration. When done is true, waiting is over and
// err is the terminal result (nil on success). When done is false, message is
// shown next to the spinner until the next tick and err is ignored.
type pollStep func(ctx context.Context) (done bool, message string, err error)

// poll runs step on a ticker until it reports done, the timeout elapses, or ctx
// is cancelled, driving the spinner shown to the user. timeoutMsg is appended to
// the timeout/cancel errors.
func poll(ctx context.Context, out io.Writer, timeout, interval time.Duration, initialMessage, timeoutMsg string, step pollStep) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	spinner := NewSpinner(out, initialMessage)
	defer spinner.Stop()

	for {
		select {
		case <-ctx.Done():
			switch {
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				return ExitWithCode(ExitTimeout, fmt.Errorf("wait timeout reached after %v - %s", timeout, timeoutMsg))
			case errors.Is(ctx.Err(), context.Canceled):
				return fmt.Errorf("canceled waiting - %s", timeoutMsg)
			default:
				return fmt.Errorf("error waiting - %s: %w", timeoutMsg, ctx.Err())
			}
		case <-ticker.C:
			done, message, err := step(ctx)
			if done {
				return err
			}
			spinner.Update(message)
		}
	}
}

func WaitForService(ctx context.Context, args WaitForServiceArgs) error {
	if done, err := args.Handler.InitialCheck(); done {
		return err
	}

	return poll(ctx, args.Output, args.Timeout, time.Second, args.Handler.Message(), args.TimeoutMsg,
		func(ctx context.Context) (bool, string, error) {
			resp, err := args.Client.GetServiceWithResponse(ctx, args.ProjectID, args.ServiceID)
			if err != nil {
				return false, fmt.Sprintf("Error checking service status: %s", err), nil
			}

			if done, err := args.Handler.Check(resp); done {
				return true, "", err
			} else if err != nil {
				return false, fmt.Sprintf("Error checking service status: %s", err), nil
			}

			return false, args.Handler.Message(), nil
		})
}

type StatusWaitHandler struct {
	TargetStatus string
	Service      *api.Service
}

func (h *StatusWaitHandler) Message() string {
	return fmt.Sprintf("Service status: %s", util.DerefStr(h.Service.Status))
}

func (h *StatusWaitHandler) InitialCheck() (bool, error) {
	// Check initial service status
	return h.checkServiceStatus(h.Service)
}

func (h *StatusWaitHandler) Check(resp *api.GetServiceResponse) (bool, error) {
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return true, errors.New("no response body returned from API")
		}

		// Update the passed-in service's status, so it's correct when output after waiting.
		h.Service.Status = resp.JSON200.Status

		// Check returned service status
		return h.checkServiceStatus(resp.JSON200)
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

func (h *StatusWaitHandler) checkServiceStatus(service *api.Service) (bool, error) {
	status := util.DerefStr(service.Status)
	switch status {
	case h.TargetStatus:
		return true, nil
	case "FAILED", "ERROR":
		return true, fmt.Errorf("service failed with status: %s", status)
	default:
		return false, nil
	}
}

// WaitForReplicaSetArgs configures WaitForReplicaSet.
type WaitForReplicaSetArgs struct {
	Client       *api.ClientWithResponses
	ProjectID    string
	ServiceID    string
	ReplicaSetID string
	Output       io.Writer
	Timeout      time.Duration
}

// WaitForReplicaSet polls until the replica set with the given ID becomes
// active, returning it. It errors if the replica enters an error state or the
// timeout is reached.
func WaitForReplicaSet(ctx context.Context, args WaitForReplicaSetArgs) (*api.ReadReplicaSet, error) {
	const initialMessage = "Read replica status: creating"

	var found *api.ReadReplicaSet
	err := poll(ctx, args.Output, args.Timeout, 2*time.Second, initialMessage, "read replica may still be provisioning",
		func(ctx context.Context) (bool, string, error) {
			resp, err := args.Client.GetReplicaSetsWithResponse(ctx, args.ProjectID, args.ServiceID)
			if err != nil {
				return false, fmt.Sprintf("Error checking read replica status: %s", err), nil
			}
			if resp.StatusCode() != 200 || resp.JSON200 == nil {
				return false, fmt.Sprintf("Error checking read replica status: %s", resp.Status()), nil
			}

			for i := range *resp.JSON200 {
				rs := &(*resp.JSON200)[i]
				if rs.Id == nil || *rs.Id != args.ReplicaSetID {
					continue
				}
				switch util.Deref(rs.Status) {
				case api.ReadReplicaSetStatusActive:
					found = rs
					return true, "", nil
				case api.ReadReplicaSetStatusError:
					return true, "", fmt.Errorf("read replica entered error state")
				default:
					return false, fmt.Sprintf("Read replica status: %s", util.Deref(rs.Status)), nil
				}
			}

			// Not in the list yet; keep showing the initial message.
			return false, initialMessage, nil
		})
	if err != nil {
		return nil, err
	}
	return found, nil
}

type DeletionWaitHandler struct {
	ServiceID string
}

func (h *DeletionWaitHandler) Message() string {
	return fmt.Sprintf("Waiting for service '%s' to be deleted", h.ServiceID)
}

func (h *DeletionWaitHandler) InitialCheck() (bool, error) {
	return false, nil
}

func (h *DeletionWaitHandler) Check(resp *api.GetServiceResponse) (bool, error) {
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
