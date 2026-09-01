package common

import (
	"context"
	"errors"
	"fmt"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
)

// ErrReadOnly is returned when read_only=all blocks a destructive operation.
var ErrReadOnly = errors.New("this operation is not allowed in read-only mode")

// ErrReadOnlyProd is returned when read_only=prod blocks a destructive operation
// on a service tagged PROD.
var ErrReadOnlyProd = errors.New(`this operation is not allowed on services tagged PROD while read_only is set to "prod"`)

// CheckReadOnly returns an error when read-only mode blocks writes to a service
// with the given environment tag. Use CheckReadOnlyByServiceID when only the ID
// is known; where a boolean is wanted, compare the result against nil.
func CheckReadOnly(cfg *config.Config, tag api.EnvironmentTag) error {
	switch cfg.ReadOnly {
	case config.ReadOnlyAll:
		return ErrReadOnly
	case config.ReadOnlyProd:
		if tag == api.EnvironmentTagPROD {
			return ErrReadOnlyProd
		}
	}
	return nil
}

// CheckReadOnlyByServiceID is CheckReadOnly for a caller that has only the
// service's ID, fetching the service to read its tag. A failed fetch is a
// refusal: we can't tell whether the service is PROD.
//
// Only prod's verdict depends on the tag, so only prod pays for the lookup.
func CheckReadOnlyByServiceID(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface, projectID, serviceID string) error {
	if cfg.ReadOnly.BlocksAll() {
		return ErrReadOnly
	}
	if cfg.ReadOnly != config.ReadOnlyProd {
		return nil
	}

	service, err := GetService(ctx, client, projectID, serviceID)
	if err != nil {
		// %w keeps any ExitCodeError in the chain, and main.go unwraps to find it,
		// so an unknown service still exits with its own code rather than 1.
		return fmt.Errorf("cannot verify whether service %s is tagged PROD (read_only=%s): %w", serviceID, cfg.ReadOnly, err)
	}

	if err := CheckReadOnly(cfg, ServiceEnvironmentTag(*service)); err != nil {
		return fmt.Errorf("service %s: %w", serviceID, err)
	}
	return nil
}
