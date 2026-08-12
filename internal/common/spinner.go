package common

import (
	"context"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/timescale/tiger-cli/internal/util"
)

// spinnerFrames defines the animation frames for the spinner
var spinnerFrames = []string{"⢎ ", "⠎⠁", "⠊⠑", "⠈⠱", " ⡱", "⢀⡰", "⢄⡠", "⢆⡀"}

type Spinner interface {
	// Update changes the spinner's displayed message.
	Update(message string)

	// Stop terminates the spinner program and waits for it to finish.
	Stop()
}

type SpinnerArgs struct {
	// Input is read so the animated spinner can see Ctrl+C. BubbleTea asks the
	// terminal for keyboard disambiguation, and terminals that honor it deliver
	// Ctrl+C as an escape sequence on stdin instead of as a SIGINT — so without
	// stdin attached the keypress has nowhere to go and Ctrl+C does nothing.
	Input io.Reader

	Output io.Writer

	Message string

	// Cancel is called when the user presses Ctrl+C. It lets the caller's
	// polling loop unwind through its own context rather than being torn down
	// from underneath.
	Cancel context.CancelFunc
}

// NewSpinner creates and returns a new [Spinner] for displaying animated
// status messages. If the output is nil or [io.Discard], it returns a no-op
// spinner. If both streams are terminals, it uses bubbletea to dynamically
// update the spinner and message in place. Otherwise it prints each message on
// a new line without animation.
func NewSpinner(args SpinnerArgs) Spinner {
	if args.Output == nil || args.Output == io.Discard {
		return newNopSpinner()
	}
	if util.IsTerminal(args.Input) && util.IsTerminal(args.Output) {
		return newAnimatedSpinner(args)
	}
	return newManualSpinner(args.Output, args.Message)
}

type nopSpinner struct{}

func newNopSpinner() nopSpinner {
	return nopSpinner{}
}

func (s nopSpinner) Update(message string) {}

func (s nopSpinner) Stop() {}

type animatedSpinner struct {
	program *tea.Program
}

func newAnimatedSpinner(args SpinnerArgs) *animatedSpinner {
	// No tea.WithContext here: the caller's polling loop owns cancellation and
	// stops the program itself, so wiring the context in as well would give the
	// same shutdown two racing owners.
	program := tea.NewProgram(
		spinnerModel{
			message: args.Message,
			cancel:  args.Cancel,
		},
		tea.WithInput(args.Input),
		tea.WithOutput(args.Output),
		tea.WithoutSignalHandler(),
	)

	go func() {
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(args.Output, "Error displaying output: %s\n", err)
		}
	}()

	return &animatedSpinner{
		program: program,
	}
}

// Update changes the spinner's displayed message and triggers bubbletea to re-render.
func (s *animatedSpinner) Update(message string) {
	s.program.Send(updateMsg(message))
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

func newManualSpinner(output io.Writer, message string) *manualSpinner {
	s := &manualSpinner{
		output: output,
		model: &spinnerModel{
			message: message,
		},
	}
	s.printLine()
	return s
}

// Update prints the message on a new line if it differs from the previous one.
func (s *manualSpinner) Update(message string) {
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
	fmt.Fprintln(s.output, s.model.render())
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
	cancel  context.CancelFunc
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
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" && m.cancel != nil {
			// Cancel the caller's work; it stops the spinner on its way out.
			m.cancel()
		}
	}
	return m, nil
}

func (m spinnerModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m spinnerModel) render() string {
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
