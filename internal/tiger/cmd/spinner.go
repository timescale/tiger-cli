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
	// Populated when output is a TTY
	program *tea.Program

	// Populated when output is not a TTY
	output io.Writer
	model  *spinnerModel
}

func NewSpinner(output io.Writer, message string, args ...any) *Spinner {
	model := spinnerModel{
		message: fmt.Sprintf(message, args...),
	}

	// If output is not a TTY, print each message on a new line
	if !isTerminal(output) {
		s := &Spinner{
			output: output,
			model:  &model,
		}
		s.println()
		return s
	}

	// If output is a TTY, use bubbletea to dynamically update the message
	program := tea.NewProgram(
		model,
		tea.WithOutput(output),
	)

	// Start the program in a goroutine
	go func() {
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(output, "Error displaying output: %s\n", err)
		}
	}()

	return &Spinner{
		program: program,
	}
}

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

func (s *Spinner) Stop() {
	if s.program == nil {
		return
	}

	s.program.Quit()
	s.program.Wait()
}

func (s *Spinner) println() {
	fmt.Fprintln(s.output, s.model.View())
}

func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// Message types for the bubbletea model
type tickMsg struct{}
type updateMsg string

// spinnerModel is the bubbletea model for the spinner
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
