package cmd

import (
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// spinnerFrames defines the animation frames for the spinner
var spinnerFrames = []string{"⢎ ", "⠎⠁", "⠊⠑", "⠈⠱", " ⡱", "⢀⡰", "⢄⡠", "⢆⡀"}

type Spinner interface {
	// Update changes the spinner's displayed message.
	Update(message string, args ...any)

	// Stop terminates the spinner program and waits for it to finish.
	Stop()
}

// NewSpinner creates and returns a new [Spinner] for displaying animated
// status messages. If output is a terminal, it uses bubbletea to dynamically
// update the spinner and message in place. If output is not a terminal, it
// prints each message on a new line without animation. The message parameter
// supports fmt.Sprintf-style formatting with optional args.
func NewSpinner(output io.Writer, message string, args ...any) Spinner {
	if util.IsTerminal(output) {
		return newAnimatedSpinner(output, message, args...)
	}
	return newManualSpinner(output, message, args...)
}

type animatedSpinner struct {
	program *tea.Program
}

func newAnimatedSpinner(output io.Writer, message string, args ...any) *animatedSpinner {
	program := tea.NewProgram(
		spinnerModel{
			message: fmt.Sprintf(message, args...),
		},
		tea.WithInput(nil),
		tea.WithOutput(output),
		tea.WithoutSignalHandler(),
	)

	go func() {
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(output, "Error displaying output: %s\n", err)
		}
	}()

	return &animatedSpinner{
		program: program,
	}
}

// Update changes the spinner's displayed message and triggers bubbletea to re-render.
func (s *animatedSpinner) Update(message string, args ...any) {
	s.program.Send(updateMsg(fmt.Sprintf(message, args...)))
}

// Stop quits the [tea.Program] and waits for it to finish.
func (s *animatedSpinner) Stop() {
	s.program.Quit()
	s.program.Wait()
}

type manualSpinner struct {
	output io.Writer
	model  *spinnerModel
}

func newManualSpinner(output io.Writer, message string, args ...any) *manualSpinner {
	s := &manualSpinner{
		output: output,
		model: &spinnerModel{
			message: fmt.Sprintf(message, args...),
		},
	}
	s.printLine()
	return s
}

// Update prints the message on a new line if it differs from the previous one.
func (s *manualSpinner) Update(message string, args ...any) {
	message = fmt.Sprintf(message, args...)
	if message == s.model.message {
		return
	}

	s.model.message = message
	s.model.incFrame()
	s.printLine()
}

// Stop is a no-op for a manual spinner.
func (s *manualSpinner) Stop() {}

func (s *manualSpinner) printLine() {
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
