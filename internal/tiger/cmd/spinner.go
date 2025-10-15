package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// spinnerFrames defines the animation frames for the spinner
var spinnerFrames = []string{"⢎ ", "⠎⠁", "⠊⠑", "⠈⠱", " ⡱", "⢀⡰", "⢄⡠", "⢆⡀"}

// spinner wraps output to show a nice spinner that updates in place
type spinner struct {
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	lock        sync.Mutex
	output      io.Writer
	tty         bool
	message     string
	frame       int
	lastLineLen int
}

func newSpinner(ctx context.Context, output io.Writer, message string, args ...any) *spinner {
	ctx, cancel := context.WithCancel(ctx)
	s := &spinner{
		ctx:     ctx,
		cancel:  cancel,
		output:  output,
		message: fmt.Sprintf(message, args...),
		done:    make(chan struct{}),
	}

	// Check if output is a TTY
	if f, ok := output.(*os.File); ok {
		s.tty = term.IsTerminal(int(f.Fd()))
	}

	// Start the spinner animation in a goroutine
	go s.run()

	return s
}

func (s *spinner) run() {
	defer close(s.done)

	// Initial render
	s.render(true)

	// If not outputting to a terminal, do not constantly re-render the spinner
	if !s.tty {
		return
	}

	// Re-render the spinner every 100ms
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			// Move to the next line when finished
			fmt.Fprintln(s.output)
			return
		case <-ticker.C:
			s.render(true)
		}
	}
}

func (s *spinner) render(incFrame bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// If outputting to a terminal, clear the previous line
	if s.tty && s.lastLineLen > 0 {
		fmt.Fprint(s.output, "\r")
		fmt.Fprint(s.output, strings.Repeat(" ", s.lastLineLen))
		fmt.Fprint(s.output, "\r")
	}

	// Build the spinner line
	spinnerFrame := spinnerFrames[s.frame]
	if incFrame {
		s.frame = (s.frame + 1) % len(spinnerFrames)
	}

	line := fmt.Sprintf("%s %s", spinnerFrame, s.message)
	s.lastLineLen = len(line)

	if s.tty {
		// If outputting to a terminal, write without newline so it stays on the same line
		fmt.Fprint(s.output, line)
	} else {
		// If not outputting to a terminal, write with a newline
		fmt.Fprintln(s.output, line)
	}
}

func (s *spinner) update(message string, args ...any) {
	s.lock.Lock()
	s.message = fmt.Sprintf(message, args...)
	s.lock.Unlock()

	// Immediately render the updated message. Only increment the spinner frame
	// if not outputting to a terminal (if outputting to a terminal, the
	// spinner frame updates on a set schedule via a time.Ticker).
	s.render(!s.tty)
}

func (s *spinner) stop() {
	// Cancel the context to stop the spinner
	s.cancel()

	// Wait for spinner goroutine to finish rendering
	<-s.done
}
