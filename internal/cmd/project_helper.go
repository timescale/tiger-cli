package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/analytics"
	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// selectProjectInteractively can be overridden for testing
var selectProjectInteractively = selectProjectInteractivelyImpl

// fetchProjects lists the projects the caller can access: every project an
// OAuth user belongs to, or the single project a PAT is scoped to.
func fetchProjects(ctx context.Context, client api.ClientWithResponsesInterface) ([]api.Project, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user projects: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	return *resp.JSON200, nil
}

// resolveProjectID picks the project to use. A requested ID (from --project-id
// or a positional argument) is validated against the accessible ones; otherwise
// a lone project is used as-is and a choice between several is made
// interactively. nonInteractiveHint names how the calling command takes a
// project ID without a prompt, for the error raised when there's no terminal.
func resolveProjectID(cmd *cobra.Command, projects []api.Project, requested, nonInteractiveHint string) (api.Project, error) {
	if requested != "" {
		for _, project := range projects {
			if project.ID == requested {
				return project, nil
			}
		}
		return api.Project{}, common.ExitWithCode(common.ExitInvalidParameters,
			analytics.RedactError(
				fmt.Errorf("project %q not found or not accessible.%s", requested, accessibleProjectsHint(projects)),
				"project not found or not accessible"))
	}

	switch len(projects) {
	case 0:
		return api.Project{}, fmt.Errorf("user has no accessible projects")
	case 1:
		return projects[0], nil
	default:
		if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
			return api.Project{}, analytics.RedactError(
				fmt.Errorf("TTY not detected - cannot select between %d projects. To choose one, %s.%s",
					len(projects), nonInteractiveHint, accessibleProjectsHint(projects)),
				fmt.Sprintf("TTY not detected - cannot select between %d projects", len(projects)))
		}

		return selectProjectInteractively(cmd, projects)
	}
}

// accessibleProjectsHint lists the caller's projects for an error message, so a
// misspelled project ID doesn't need a second command to fix.
func accessibleProjectsHint(projects []api.Project) string {
	if len(projects) == 0 {
		return ""
	}

	described := make([]string, 0, len(projects))
	for _, project := range projects {
		described = append(described, describeProject(project))
	}
	return " Accessible projects: " + strings.Join(described, ", ")
}

// describeProject renders a project for human-readable output.
func describeProject(project api.Project) string {
	if project.Name == "" {
		return project.ID
	}
	return fmt.Sprintf("%s (%s)", project.Name, project.ID)
}

// selectProjectInteractivelyImpl is the default implementation for project selection using Bubble Tea
func selectProjectInteractivelyImpl(cmd *cobra.Command, projects []api.Project) (api.Project, error) {
	model := projectSelectModel{
		projects: projects,
		cursor:   0,
	}

	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
	finalModel, err := program.Run()
	if err != nil {
		return api.Project{}, fmt.Errorf("failed to run project selection: %w", err)
	}

	result := finalModel.(projectSelectModel)
	if result.selected.ID == "" {
		return api.Project{}, fmt.Errorf("no project selected")
	}

	return result.selected, nil
}

type projectSelectModel struct {
	projects     []api.Project
	cursor       int
	selected     api.Project
	numberBuffer string
}

func (m projectSelectModel) Init() tea.Cmd {
	return nil
}

func (m projectSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.selected = m.projects[m.cursor]
			return m, tea.Quit
		case "backspace":
			// Handle backspace to remove last character from buffer
			if len(m.numberBuffer) > 0 {
				m.updateNumberBuffer(m.numberBuffer[:len(m.numberBuffer)-1])
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Add digit to buffer and update cursor position
			m.updateNumberBuffer(m.numberBuffer + msg.String())
		case "ctrl+w", "esc":
			// Clear buffer on escape
			m.numberBuffer = ""
		}
	}
	return m, nil
}

// updateNumberBuffer moves the cursor to the project matching the number buffer
func (m *projectSelectModel) updateNumberBuffer(newBuffer string) {
	if newBuffer == "" {
		m.numberBuffer = newBuffer
		return
	}

	// Parse the buffer as a number
	num, err := strconv.Atoi(newBuffer)
	if err != nil {
		return
	}

	// Convert from 1-based to 0-based index and validate bounds
	index := num - 1
	if index >= 0 && index < len(m.projects) {
		m.numberBuffer = newBuffer
		m.cursor = index
	}
}

func (m projectSelectModel) View() tea.View {
	s := "Select a project:\n\n"

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s (%s)\n", cursor, i+1, project.Name, project.ID)
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s += fmt.Sprintf("\nTyping: %s", m.numberBuffer)
	}

	s += "\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit"
	return tea.NewView(s)
}
