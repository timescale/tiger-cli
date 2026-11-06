package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

// pickerItem is one row of the checkbox picker.
type pickerItem struct {
	label    string
	detail   string // shown dimmed after the label
	selected bool
}

// runPicker runs the interactive checkbox picker over items, below header,
// and returns which of them the user chose. The caller must check that stdin
// and stderr are terminals first. It's a var so command tests can stub the
// interactive part.
var runPicker = func(cmd *cobra.Command, header string, items []pickerItem) ([]bool, error) {
	model := pickerModel{
		header: header,
		items:  items,
	}
	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
	final, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run selection menu: %w", err)
	}

	result := final.(pickerModel)
	if !result.confirmed {
		return nil, errors.New("installation cancelled")
	}
	chosen := make([]bool, len(result.items))
	for i, item := range result.items {
		chosen[i] = item.selected
	}
	return chosen, nil
}

// pickerModel is the Bubble Tea model for the checkbox picker.
type pickerModel struct {
	header    string
	items     []pickerItem
	cursor    int
	confirmed bool
	done      bool // set on quitting, however that happens, to clear the picker
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch k := key.String(); k {
	case "ctrl+c", "q", "esc":
		m.done = true
		return m, tea.Quit
	case "enter":
		m.confirmed = true
		m.done = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "space":
		m.items[m.cursor].selected = !m.items[m.cursor].selected
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(k[0] - '1'); i < len(m.items) {
			m.cursor = i
			m.items[i].selected = !m.items[i].selected
		}
	}
	return m, nil
}

// Styles for the checkbox picker, taken from the terminal theme. Bright colors
// and bold are avoided because some themes render them as greys, and dim text
// uses the faint attribute so it suits light and dark themes alike.
var (
	pickerCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Blue)
	pickerCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	pickerDimStyle    = lipgloss.NewStyle().Faint(true)
)

func (m pickerModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	width := 0
	for _, item := range m.items {
		width = max(width, len(item.label))
	}

	var b strings.Builder
	b.WriteString(m.header + "\n\n")
	for i, item := range m.items {
		cursor := " "
		if m.cursor == i {
			cursor = pickerCursorStyle.Render(">")
		}
		check := " "
		if item.selected {
			check = pickerCheckStyle.Render("✓")
		}
		fmt.Fprintf(&b, "%s [%s] %d. %-*s  %s\n", cursor, check, i+1, width, item.label, pickerDimStyle.Render(item.detail))
	}
	b.WriteString("\n" + pickerDimStyle.Render("Use ↑/↓ to navigate, space or number keys to toggle, enter to confirm, q to quit"))
	return tea.NewView(b.String())
}

// displayPath abbreviates the home directory in path to ~.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rel, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rel)
	}
	return path
}
