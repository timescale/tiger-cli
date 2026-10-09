package util

import (
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// IsTerminal is a helper for detecting whether an [io.Writer] or [io.Reader]
// is an interactive terminal / TTY. It is a variable so that tests can
// override it.
var IsTerminal = func(v any) bool {
	if tw, ok := v.(*TermWriter); ok {
		return term.IsTerminal(int(tw.Fd()))
	}
	if f, ok := v.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// TermWriter embeds *os.File so that Fd(), Read(), and Close() are promoted
// automatically (satisfying bubbletea's term.File interface for terminal
// detection), while overriding Write() to route through a colorprofile.Writer
// for automatic color downsampling.
type TermWriter struct {
	*os.File
	ColorProfile *colorprofile.Writer
}

// NewTermWriter creates a TermWriter that writes through a colorprofile.Writer
// while exposing the underlying file's terminal capabilities.
func NewTermWriter(f *os.File) *TermWriter {
	return &TermWriter{
		File:         f,
		ColorProfile: colorprofile.NewWriter(f, os.Environ()),
	}
}

func (tw *TermWriter) Write(p []byte) (int, error) {
	return tw.ColorProfile.Write(p)
}

// TryUnwrapFile attempts to extract the underlying *os.File from a writer.
// If the writer is a *TermWriter, the embedded *os.File is returned; otherwise
// the original writer is returned unchanged. This is needed when passing writers to exec.Cmd, since
// os/exec only passes file descriptors directly to child processes when the
// writer is an *os.File. Any other io.Writer causes os/exec to create a pipe,
// which breaks TTY detection in the child process.
func TryUnwrapFile(w io.Writer) io.Writer {
	if tw, ok := w.(*TermWriter); ok {
		return tw.File
	}
	return w
}
