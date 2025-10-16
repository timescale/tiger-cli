package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// spinnerFrames defines the animation frames for the spinner
var spinnerFrames = []string{"⢎ ", "⠎⠁", "⠊⠑", "⠈⠱", " ⡱", "⢀⡰", "⢄⡠", "⢆⡀"}

type Spinner struct {
	// Populated when output is a terminal
	program *tea.Program

	// Populated when output is not a terminal
	output io.Writer
	model  *spinnerModel
}

// NewSpinner creates and returns a new [Spinner] for displaying animated
// status messages. If output is a terminal, it uses bubbletea to dynamically
// update the spinner in place. If output is not a terminal, it prints each
// message on a new line without animation. The message parameter supports
// fmt.Sprintf-style formatting with optional args.
func NewSpinner(output io.Writer, message string, args ...any) *Spinner {
	if isTerminal(output) {
		return newAnimatedSpinner(output, message, args...)
	}
	return newManualSpinner(output, message, args...)
}

// isTerminal is a helper method for detecting whether an [io.Writer] is a
// terminal.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func newAnimatedSpinner(output io.Writer, message string, args ...any) *Spinner {
	program := tea.NewProgram(
		spinnerModel{
			message: fmt.Sprintf(message, args...),
		},
		tea.WithOutput(output),
	)

	go func() {
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(output, "Error displaying output: %s\n", err)
		}
	}()

	return &Spinner{
		program: program,
	}
}

func newManualSpinner(output io.Writer, message string, args ...any) *Spinner {
	s := &Spinner{
		output: output,
		model: &spinnerModel{
			message: fmt.Sprintf(message, args...),
		},
	}
	s.println()
	return s
}

// Update changes the spinner's displayed message. If the output is a terminal,
// the message is updated in place via bubbletea. Otherwise, the message is
// printed on a new line if it differs from the previous one.
func (s *Spinner) Update(message string, args ...any) {
	message = fmt.Sprintf(message, args...)
	if s.program != nil {
		s.program.Send(updateMsg(message))
	} else if message != s.model.message {
		s.model.message = message
		s.model.incFrame()
		s.println()
	}
}

// Stop terminates the spinner program and waits for it to finish.
// This method is a no-op if the output is not a terminal.
func (s *Spinner) Stop() {
	if s.program == nil {
		return
	}

	s.program.Quit()
	s.program.Wait()
}

// println prints the current state of the model to the configured output on a
// new line. It is used when the output is not a terminal, and we therefore
// don't want to write terminal control characters.
func (s *Spinner) println() {
	fmt.Fprintln(s.output, s.model.View())
}

// Message types for the [tea.Model].
type (
	tickMsg   struct{}
	updateMsg string
)

// spinnerModel is the [tea.Model] for the spinner.
type spinnerModel struct {
	message string
	frame   int
}

func (m spinnerModel) Init() tea.Cmd {
	return m.tick()
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.incFrame()
		return m, m.tick()
	case updateMsg:
		m.message = string(msg)
	}
	return m, nil
}

func (m spinnerModel) View() string {
	return fmt.Sprintf("%s %s", spinnerFrames[m.frame], m.message)
}

func (m *spinnerModel) incFrame() {
	m.frame = (m.frame + 1) % len(spinnerFrames)
}

func (m *spinnerModel) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}
