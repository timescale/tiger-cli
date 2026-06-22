package common

import (
	"errors"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

var (
	// ErrPaused is returned for a paused (or pausing) service.
	ErrPaused = errors.New("service is paused")

	// ErrNotReady is returned for a service that isn't accepting connections
	// (provisioning, resuming, upgrading, deleting, etc.).
	ErrNotReady = errors.New("service is not ready")
)

// CheckServiceReady returns nil only when the service is READY, ErrPaused for
// PAUSED/PAUSING, and ErrNotReady for every other (or unknown) status.
func CheckServiceReady(service api.Service) error {
	if service.Status == nil {
		return ErrNotReady
	}
	switch *service.Status {
	case api.READY:
		return nil
	case api.PAUSED, api.PAUSING:
		return ErrPaused
	default:
		return ErrNotReady
	}
}
