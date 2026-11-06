package common

import (
	"errors"

	"github.com/timescale/tiger-cli/internal/api"
)

var (
	// ErrPaused is returned for a paused (or pausing) service.
	ErrPaused = errors.New("service is paused")

	// ErrNotReady is returned for a service that isn't accepting connections
	// (provisioning, resuming, upgrading, deleting, etc.).
	ErrNotReady = errors.New("service is not ready")
)

// CheckReady returns nil only for a READY status, ErrPaused for
// PAUSED/PAUSING, and ErrNotReady for every other (or unknown) status.
func CheckReady(status api.DeployStatus) error {
	switch status {
	case api.DeployStatusREADY:
		return nil
	case api.DeployStatusPAUSED, api.DeployStatusPAUSING:
		return ErrPaused
	default:
		return ErrNotReady
	}
}
