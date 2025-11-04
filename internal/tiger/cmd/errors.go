package cmd

import "errors"

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
	ExitConflict            = 8 // Resource conflict (e.g., service cannot be modified in current state)
)

// exitCodeError creates an error that will cause the program to exit with the specified code
type exitCodeError struct {
	code int
	err  error
}

func (e exitCodeError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e exitCodeError) ExitCode() int {
	return e.code
}

// exitWithCode returns an error that will cause the program to exit with the specified code
func exitWithCode(code int, err error) error {
	return exitCodeError{code: code, err: err}
}

// exitWithErrorFromStatusCode maps HTTP status codes to CLI exit codes
func exitWithErrorFromStatusCode(statusCode int, err error) error {
	if err == nil {
		err = errors.New("unknown error")
	}
	switch statusCode {
	case 400:
		// Bad request - invalid parameters
		return exitWithCode(ExitInvalidParameters, err)
	case 401:
		// Unauthorized - authentication error
		return exitWithCode(ExitAuthenticationError, err)
	case 403:
		// Forbidden - permission denied
		return exitWithCode(ExitPermissionDenied, err)
	case 404:
		// Not found - service/resource not found
		return exitWithCode(ExitServiceNotFound, err)
	case 408, 504:
		// Request timeout or gateway timeout
		return exitWithCode(ExitTimeout, err)
	default:
		// For other 4xx errors, use general error
		if statusCode >= 400 && statusCode < 500 {
			return exitWithCode(ExitGeneralError, err)
		}
		// For 5xx and other errors, use general error
		return exitWithCode(ExitGeneralError, err)
	}
}
