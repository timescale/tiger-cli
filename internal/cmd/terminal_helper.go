package cmd

import (
	"context"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/timescale/tiger-cli/internal/util"
)

var (
	// checkStdinIsTTY can be overridden for testing to bypass TTY detection
	checkStdinIsTTY = func() bool {
		return util.IsTerminal(os.Stdin)
	}

	// readPasswordFromTerminal can be overridden for testing to inject password input
	readPasswordFromTerminal = func() (string, error) {
		val, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", err
		}
		return string(val), nil
	}
)

func readString(ctx context.Context, readFn func() (string, error)) (string, error) {
	valCh := make(chan string)
	errCh := make(chan error)
	defer func() { close(valCh); close(errCh) }()
	go func() {
		val, err := readFn()
		if err != nil {
			errCh <- err
			return
		}
		select {
		case <-ctx.Done(): // don't return an empty value if the context is already canceled
			return
		default:
		}
		valCh <- val
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case val := <-valCh:
		return strings.TrimSpace(val), nil
	}
}
