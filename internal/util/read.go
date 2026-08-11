package util

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ReadPassword reads a password from stdin without echoing it. It is a
// variable so that tests can inject input (similar to IsTerminal), since the
// real implementation requires stdin to be an *os.File.
var ReadPassword = func(ctx context.Context, stdin io.Reader) (string, error) {
	// If stdin is a terminal, save its state so we can restore it on context cancellation.
	// This handles cases like term.ReadPassword which puts the terminal in raw mode -
	// if the user hits Ctrl+C, we want to restore the terminal to its original state.
	f, ok := stdin.(*os.File)
	if !ok {
		return "", errors.New("stdin is not a terminal")
	}

	state, err := term.GetState(int(f.Fd()))
	if err != nil {
		return "", fmt.Errorf("error getting current terminal state: %w", err)
	}
	defer term.Restore(int(f.Fd()), state)

	return readString(ctx, func() (string, error) {
		val, err := term.ReadPassword(int(f.Fd()))
		if err != nil {
			return "", err
		}
		return string(val), nil
	})
}

func ReadAll(ctx context.Context, stdin io.Reader) (string, error) {
	return readString(ctx, func() (string, error) {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
}

func ReadLine(ctx context.Context, stdin io.Reader) (string, error) {
	return readString(ctx, func() (string, error) {
		reader := bufio.NewReader(stdin)
		return reader.ReadString('\n')
	})
}

type readStringFn func() (string, error)

// readString makes it possible to perform a blocking read from stdin while
// still respecting context cancellation.
func readString(ctx context.Context, readFn readStringFn) (string, error) {
	type result struct {
		val string
		err error
	}

	resultCh := make(chan result, 1)
	go func() {
		defer close(resultCh)

		val, err := readFn()
		if err != nil {
			resultCh <- result{err: err}
			return
		}

		// Don't return a value if the context is already canceled
		if ctx.Err() != nil {
			return
		}
		resultCh <- result{val: val}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-resultCh:
		return strings.TrimSpace(result.val), result.err
	}
}
