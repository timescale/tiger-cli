package common

import (
	"errors"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// ErrReadOnly is returned when a destructive operation is attempted while
// read-only mode is enabled in the user's config.
var ErrReadOnly = errors.New("this operation is not allowed in read-only mode")

// CheckReadOnly returns ErrReadOnly if read-only mode is enabled. Callers
// should invoke this before any destructive API call.
func CheckReadOnly(cfg *config.Config) error {
	if cfg.ReadOnly {
		return ErrReadOnly
	}
	return nil
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

// IsNotFound reports whether err represents an HTTP 404 "not found" API failure,
// as produced by ExitWithErrorFromStatusCode.
func IsNotFound(err error) bool {
	var exitErr ExitCodeError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == ExitServiceNotFound
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
