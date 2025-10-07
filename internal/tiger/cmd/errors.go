package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

// Exit codes as defined in the CLI specification
const (
	ExitSuccess             = 0 // Success
	ExitGeneralError        = 1 // General error
	ExitTimeout             = 2 // Operation timeout (wait-timeout exceeded) or connection timeout
	ExitInvalidParameters   = 3 // Invalid parameters
	ExitAuthenticationError = 4 // Authentication error
	ExitPermissionDenied    = 5 // Permission denied
	ExitServiceNotFound     = 6 // Service not found
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

// formatAPIError creates an error message from an API error response.
// If the API error contains a message, it will be used; otherwise the fallback message is returned.
func formatAPIError(apiErr *api.Error, fallback string) error {
	if apiErr != nil && apiErr.Message != nil && *apiErr.Message != "" {
		return fmt.Errorf("%s", *apiErr.Message)
	}
	return fmt.Errorf("%s", fallback)
}

// formatAPIErrorFromBody attempts to parse an API error from a response body.
// If the body contains a valid API error with a message, it will be used; otherwise the fallback message is returned.
func formatAPIErrorFromBody(body []byte, fallback string) error {
	if len(body) > 0 {
		var apiErr api.Error
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if apiErr.Message != nil && *apiErr.Message != "" {
				return fmt.Errorf("%s", *apiErr.Message)
			}
		}
	}
	return fmt.Errorf("%s", fallback)
}
