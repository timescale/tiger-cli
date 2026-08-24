package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
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

// resolveProjectID picks the project to use: a requested ID (from --project-id
// or a positional argument) is validated against the accessible ones, a lone
// project is used as-is, and several prompt interactively. nonInteractiveHint
// names the calling command's promptless alternative for the no-TTY error.
func resolveProjectID(cmd *cobra.Command, projects []api.Project, requested, nonInteractiveHint string) (api.Project, error) {
	if requested != "" {
		for _, project := range projects {
			if project.ID == requested {
				return project, nil
			}
		}
		// No project ID in the message — analytics records error text verbatim,
		// and the user just typed the ID themselves.
		return api.Project{}, common.ExitWithCode(common.ExitInvalidParameters,
			errors.New("requested project not found or not accessible"))
	}

	switch len(projects) {
	case 0:
		return api.Project{}, fmt.Errorf("user has no accessible projects")
	case 1:
		return projects[0], nil
	default:
		if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
			return api.Project{}, fmt.Errorf("TTY not detected - cannot select between %d projects. To choose one, %s",
				len(projects), nonInteractiveHint)
		}

		return selectProjectInteractively(cmd, projects)
	}
}

// clearStaleDefaultService unsets the config-file service_id when the stored
// login moves between projects — a default service belongs to the project it
// was set in. Only the config file is touched, so a flag or env var still
// wins; failures only warn, since the project change has already persisted.
func clearStaleDefaultService(cmd *cobra.Command, cfg *config.Config, previousProjectID, newProjectID string) {
	if previousProjectID == "" || previousProjectID == newProjectID {
		return
	}

	// An empty flag can hide a config-file default, so Unset runs regardless.
	previous := cfg.ServiceID
	if err := cfg.Unset("service_id"); err != nil {
		cmd.PrintErrf("⚠️  Failed to clear the default service, which belongs to the previous project: %s\n", err)
		return
	}
	switch {
	case cfg.ServiceID != "":
		cmd.PrintErrf("⚠️  Default service '%s' comes from a flag or environment variable and still points at the previous project.\n", cfg.ServiceID)
	case previous != "":
		cmd.PrintErrf("🎯 Cleared default service '%s' - it belongs to the previous project.\n", previous)
	}
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
			m.numberBuffer = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			m.numberBuffer = ""
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.selected = m.projects[m.cursor]
			return m, tea.Quit
		case "backspace":
			if len(m.numberBuffer) > 0 {
				m.updateNumberBuffer(m.numberBuffer[:len(m.numberBuffer)-1])
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			m.updateNumberBuffer(m.numberBuffer + msg.String())
		case "ctrl+w", "esc":
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

	num, err := strconv.Atoi(newBuffer)
	if err != nil {
		return
	}

	// The list is displayed 1-based
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

	if m.numberBuffer != "" {
		s += fmt.Sprintf("\nTyping: %s", m.numberBuffer)
	}

	s += "\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit"
	return tea.NewView(s)
}
