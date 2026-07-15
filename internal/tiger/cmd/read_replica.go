package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// resolveConnectTarget prompts the user to connect to a read replica instead of
// the primary, returning the chosen target (nil if cancelled). It returns the
// primary without prompting when there's nothing to choose: non-interactive
// stdin, prompting disabled, or no connectable replicas.
func resolveConnectTarget(
	ctx context.Context,
	cmd *cobra.Command,
	client *api.ClientWithResponses,
	projectID string,
	primary api.Service,
	opts common.ConnectionDetailsOptions,
	noReplicaPrompt bool,
) (*common.ConnectionDetails, error) {
	returnPrimary := func() (*common.ConnectionDetails, error) {
		return buildConnectionDetailsForTarget(cmd, primary, nil, opts)
	}

	// Only prompt on an interactive terminal.
	if noReplicaPrompt || !checkStdinIsTTY() {
		return returnPrimary()
	}

	replicas, err := fetchReplicaSets(ctx, client, projectID, util.DerefStr(primary.ServiceId))
	if err != nil {
		// Don't block the connection if we can't list replicas.
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not list read replicas: %v\n", err)
		return returnPrimary()
	}

	// Only active replicas with an endpoint are connectable.
	var connectable []api.ReadReplicaSet
	for _, r := range replicas {
		if r.Status != nil && *r.Status == api.ReadReplicaSetStatusActive && r.Endpoint != nil {
			connectable = append(connectable, r)
		}
	}

	// Nothing to choose between, so skip the menu and use the primary.
	if len(connectable) == 0 {
		return returnPrimary()
	}

	choice, err := selectConnectTargetOption(cmd.ErrOrStderr(), primary, connectable)
	if err != nil {
		return nil, err
	}

	switch choice.kind {
	case targetCancel:
		return nil, nil
	case targetPrimary:
		return returnPrimary()
	}

	replica := choice.replica
	details, err := buildConnectionDetailsForTarget(cmd, primary, replica, opts)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to read replica '%s'...\n", util.DerefStr(replica.Name))
	return details, nil
}

// fetchReplicaSets retrieves the read replica sets for a service.
func fetchReplicaSets(ctx context.Context, client *api.ClientWithResponses, projectID, serviceID string) ([]api.ReadReplicaSet, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.GetReplicaSetsWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, nil
	}

	return *resp.JSON200, nil
}

// connectTargetKind enumerates the choices in the connect target menu.
type connectTargetKind int

const (
	targetPrimary connectTargetKind = iota
	targetReplica
	targetCancel
)

// connectTargetChoice is one menu entry: its display label and the action it represents.
type connectTargetChoice struct {
	kind    connectTargetKind
	replica *api.ReadReplicaSet
	label   string
}

// connectTargetModel is the Bubble Tea model for selecting a connection target.
type connectTargetModel struct {
	choices []connectTargetChoice
	cursor  int
	chosen  connectTargetChoice
}

func newConnectTargetModel(primary api.Service, replicas []api.ReadReplicaSet) connectTargetModel {
	choices := []connectTargetChoice{{
		kind:  targetPrimary,
		label: fmt.Sprintf("Connect to primary service (%s)", util.DerefStr(primary.ServiceId)),
	}}

	for i := range replicas {
		choices = append(choices, connectTargetChoice{
			kind:    targetReplica,
			replica: &replicas[i],
			label:   fmt.Sprintf("Connect to read replica '%s'", util.DerefStr(replicas[i].Name)),
		})
	}

	choices = append(choices, connectTargetChoice{kind: targetCancel, label: "Cancel"})

	return connectTargetModel{
		choices: choices,
		// Default to cancel so quitting (ctrl+c/q) is a no-op connection.
		chosen: connectTargetChoice{kind: targetCancel},
	}
}

func (m connectTargetModel) Init() tea.Cmd {
	return nil
}

func (m connectTargetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch key := msg.String(); key {
		case "ctrl+c", "q":
			m.chosen = connectTargetChoice{kind: targetCancel}
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.chosen = m.choices[m.cursor]
			return m, tea.Quit
		default:
			// Number keys jump straight to that option ('1' -> first, etc.).
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				if idx := int(key[0] - '1'); idx < len(m.choices) {
					m.cursor = idx
					m.chosen = m.choices[idx]
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m connectTargetModel) View() string {
	s := "How would you like to connect?\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, choice.label)
	}

	s += "\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to cancel"
	return s
}

// selectConnectTargetOption shows the interactive menu for choosing a
// connection target.
func selectConnectTargetOption(out io.Writer, primary api.Service, replicas []api.ReadReplicaSet) (connectTargetChoice, error) {
	model := newConnectTargetModel(primary, replicas)

	program := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return connectTargetChoice{kind: targetCancel}, fmt.Errorf("failed to run connect menu: %w", err)
	}

	return finalModel.(connectTargetModel).chosen, nil
}
