package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Ctrl+C reaches the spinner as a key press rather than a signal once
// BubbleTea turns on keyboard disambiguation, so the model has to translate it
// into cancellation itself.
func TestSpinnerModelCancelsOnCtrlC(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	m := spinnerModel{message: "Waiting", cancel: cancel}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	select {
	case <-ctx.Done():
	default:
		t.Error("expected ctrl+c to cancel the context")
	}
}

func TestSpinnerModelIgnoresOtherKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	m := spinnerModel{message: "Waiting", cancel: cancel}
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	select {
	case <-ctx.Done():
		t.Error("expected 'q' to leave the context alone")
	default:
	}
}

func TestNewSpinnerKind(t *testing.T) {
	// Neither stream is a terminal under test, so the animated spinner is never
	// selected here.
	testCases := []struct {
		name   string
		output io.Writer
		want   Spinner
	}{
		{"nil output", nil, nopSpinner{}},
		{"discarded output", io.Discard, nopSpinner{}},
		{"non-terminal output", new(bytes.Buffer), &manualSpinner{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSpinner(SpinnerArgs{Output: tc.output, Message: "Waiting"})
			if got, want := fmt.Sprintf("%T", s), fmt.Sprintf("%T", tc.want); got != want {
				t.Errorf("expected %s, got %s", want, got)
			}
		})
	}
}

// The manual spinner prints each new message on its own line, since it can't
// update in place.
func TestManualSpinnerPrintsEachMessage(t *testing.T) {
	buf := new(bytes.Buffer)
	s := NewSpinner(SpinnerArgs{Output: buf, Message: "Waiting"})
	s.Update("Waiting")
	s.Update("Still waiting")
	s.Stop()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	if !strings.HasSuffix(lines[0], "Waiting") || !strings.HasSuffix(lines[1], "Still waiting") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}
