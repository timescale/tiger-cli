package cmd

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