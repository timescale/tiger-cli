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
// with the given environment tag. When only the service's ID is known, use CheckReadOnlyByServiceID.
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
// service's ID, fetching the service under read_only=prod to read its tag. A
// failed fetch is a refusal: we can't tell whether the service is PROD, and
// refusing is the safe direction.
func CheckReadOnlyByServiceID(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface, projectID, serviceID string) error {
	if cfg.ReadOnly.BlocksAll() {
		return ErrReadOnly
	}
	// Only prod's verdict depends on the tag, so only prod pays for the lookup.
	if cfg.ReadOnly != config.ReadOnlyProd {
		return nil
	}

	service, err := GetService(ctx, client, projectID, serviceID)
	if err != nil {
		refusal := fmt.Errorf("cannot verify whether service %s is tagged PROD (read_only=%s): %w", serviceID, cfg.ReadOnly, err)

		// main.go type-asserts on ExitCode() instead of using errors.As, so the
		// wrap above would downgrade an unknown service to exit 1.
		var exitErr ExitCodeError
		if errors.As(err, &exitErr) {
			return ExitWithCode(exitErr.ExitCode(), refusal)
		}
		return refusal
	}

	if err := CheckReadOnly(cfg, ServiceEnvironmentTag(*service)); err != nil {
		return fmt.Errorf("service %s: %w", serviceID, err)
	}
	return nil
}

// ForcesReadOnlySession reports whether read-only mode requires a database
// session against this service to open in Tiger Cloud's immutable read-only mode.
func ForcesReadOnlySession(cfg *config.Config, service api.Service) bool {
	return CheckReadOnly(cfg, ServiceEnvironmentTag(service)) != nil
}

// Exit codes as defined in the CLI specification
const (
	ExitSuccess             = 0 // Success
	ExitGeneralError        = 1 // General error
	ExitTimeout             = 2 // Operation timeout (wait-timeout exceeded) or connection timeout
	ExitInvalidParameters   = 3 // Invalid parameters
	ExitAuthenticationError = 4 // Authentication error
	ExitPermissionDenied    = 5 // Permission denied
	ExitServiceNotFound     = 6 // Service not found
	ExitUpdateAvailable     = 7 // Update available
)

// ExitCodeError creates an error that will cause the program to exit with the specified code
type ExitCodeError struct {
	code int
	err  error
}

func (e ExitCodeError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e ExitCodeError) ExitCode() int {
	return e.code
}

func (e ExitCodeError) Unwrap() error {
	return e.err
}

// ExitWithCode returns an error that will cause the program to exit with the specified code
func ExitWithCode(code int, err error) error {
	return ExitCodeError{code: code, err: err}
}

// ExitWithErrorFromStatusCode maps HTTP status codes to CLI exit codes
func ExitWithErrorFromStatusCode(statusCode int, err error) error {
	if err == nil {
		err = errors.New("unknown error")
	}
	switch statusCode {
	case 400:
		// Bad request - invalid parameters
		return ExitWithCode(ExitInvalidParameters, err)
	case 401:
		// Unauthorized - authentication error
		return ExitWithCode(ExitAuthenticationError, err)
	case 403:
		// Forbidden - permission denied
		return ExitWithCode(ExitPermissionDenied, err)
	case 404:
		// Not found - service/resource not found
		return ExitWithCode(ExitServiceNotFound, err)
	case 408, 504:
		// Request timeout or gateway timeout
		return ExitWithCode(ExitTimeout, err)
	default:
		// For other 4xx errors, use general error
		if statusCode >= 400 && statusCode < 500 {
			return ExitWithCode(ExitGeneralError, err)
		}
		// For 5xx and other errors, use general error
		return ExitWithCode(ExitGeneralError, err)
	}
}
