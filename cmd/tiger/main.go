package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/timescale/tiger-cli/internal/cmd"
	"github.com/timescale/tiger-cli/internal/common"
)

func main() {
	if err := run(); err != nil {
		// A common.ExitCodeError anywhere in the chain sets the exit code;
		// foreign carriers (psql's *exec.ExitError) are converted to it at
		// their call sites (see launchPsql).
		var codeErr common.ExitCodeError
		if errors.As(err, &codeErr) {
			os.Exit(codeErr.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func run() (err error) {
	ctx, cancel := notifyContext(context.Background())
	defer func() {
		cancel()
		if r := recover(); r != nil {
			err = errors.Join(err, fmt.Errorf("panic: %v", r))
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
		}
	}()
	err = cmd.Execute(ctx)
	return
}

// noifyContext sets up graceful shutdown handling and returns a context and
// cleanup function. This is nearly identical to [signal.NotifyContext], except
// that it also restores the default signal handling behavior.
func notifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			signal.Stop(sigChan) // Restore default signal handling behavior
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		cancel()
		signal.Stop(sigChan)
	}
}
