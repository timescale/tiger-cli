package cmd

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
