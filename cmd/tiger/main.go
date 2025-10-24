package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/cmd"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

func main() {
	if err := run(); err != nil {
		// Check if it's a custom exit code error
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(exitErr.ExitCode())
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
// that it logs a message when a signal is received and also restores the default
// signal handling behavior.
func notifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigChan:
			logging.Info("Received interrupt signal, press control-C again to exit", zap.Stringer("signal", sig))
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
