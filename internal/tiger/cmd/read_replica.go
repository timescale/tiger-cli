package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// replicaCreateWaitTimeout is how long we wait for a newly created read replica
// to become active before giving up.
const replicaCreateWaitTimeout = 30 * time.Minute

// endpointReadyTimeout caps how long we wait for a replica's endpoint to start
// accepting connections. A replica can report "active" a few seconds before its
// endpoint is listening, which would otherwise look like "connection refused".
const endpointReadyTimeout = 3 * time.Minute

// resolveConnectTarget optionally prompts the user to connect to a read replica
// instead of the primary. It returns the chosen target, or (nil, nil) if the
// user cancelled. It returns the primary without prompting when stdin is not a
// TTY, when prompting is disabled, or when replicas can't be listed.
func resolveConnectTarget(
	ctx context.Context,
	cmd *cobra.Command,
	client *api.ClientWithResponses,
	projectID string,
	primary api.Service,
	opts common.ConnectionDetailsOptions,
	noReplicaPrompt bool,
	readOnly bool,
) (*common.ConnectionDetails, error) {
	// returnPrimary builds the primary's details, applying the historical pooler
	// policy: when --pooled is requested but unavailable, it's a hard error.
	returnPrimary := func() (*common.ConnectionDetails, error) {
		details, err := common.GetConnectionDetails(primary, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to build connection string: %w", err)
		}
		if opts.Pooled && !details.IsPooler {
			return nil, fmt.Errorf("connection pooler not available for this service")
		}
		return details, nil
	}

	// Only prompt in an interactive terminal, and only when not disabled.
	if noReplicaPrompt || !checkStdinIsTTY() {
		return returnPrimary()
	}

	replicas, err := fetchReplicaSets(ctx, client, projectID, util.DerefStr(primary.ServiceId))
	if err != nil {
		// Don't block the connection if we can't list replicas.
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not list read replicas: %v\n", err)
		return returnPrimary()
	}

	// Only offer connecting to replicas that are active and have an endpoint.
	var connectable []api.ReadReplicaSet
	for _, r := range replicas {
		if r.Status != nil && *r.Status == api.ReadReplicaSetStatusActive && r.Endpoint != nil {
			connectable = append(connectable, r)
		}
	}

	// Creating a read replica is a mutating operation, so it's only offered when
	// not in read-only mode.
	allowCreate := !readOnly

	// If there's no replica to connect to and creation isn't offered, the menu
	// would only contain the primary, so connect to it directly.
	if len(connectable) == 0 && !allowCreate {
		return returnPrimary()
	}

	choice, err := selectConnectTargetOption(cmd.ErrOrStderr(), primary, connectable, allowCreate)
	if err != nil {
		return nil, err
	}

	// Resolve the chosen replica (selecting an existing one or creating a new
	// one), then connect to it via the shared replica path below.
	var replica *api.ReadReplicaSet
	switch choice.kind {
	case targetCancel:
		return nil, nil
	case targetPrimary:
		return returnPrimary()
	case targetReplica:
		replica = choice.replica
	case targetCreate:
		if replica, err = createReplicaSet(ctx, cmd, client, projectID, primary); err != nil {
			return nil, err
		}
	}

	// --pooled is best-effort on replicas: a created replica never has a pooler,
	// and an existing one may not either, so warn and connect directly.
	if opts.Pooled && !replicaHasPooler(replica) {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: read replica '%s' has no connection pooler; connecting directly instead.\n", util.DerefStr(replica.Name))
		opts.Pooled = false
	}

	details, err := common.GetReplicaConnectionDetails(primary, *replica, opts)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to read replica '%s'...\n", util.DerefStr(replica.Name))

	// A replica can report active shortly before its endpoint accepts
	// connections; wait for reachability before handing off to psql (see
	// endpointReadyTimeout). Best-effort: proceed and let psql surface any error.
	if err := waitForEndpointReady(ctx, cmd.ErrOrStderr(), details.Host, details.Port, endpointReadyTimeout); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: %v\n", err)
	}
	return details, nil
}

// replicaHasPooler reports whether a replica set exposes a pooler endpoint.
func replicaHasPooler(replica *api.ReadReplicaSet) bool {
	return replica != nil && replica.ConnectionPooler != nil && replica.ConnectionPooler.Endpoint != nil
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

// createReplicaSet creates a new read replica for the primary service,
// inheriting the primary's resource allocation, and waits for it to become
// active so it can be connected to.
func createReplicaSet(ctx context.Context, cmd *cobra.Command, client *api.ClientWithResponses, projectID string, primary api.Service) (*api.ReadReplicaSet, error) {
	cpuMillis, memoryGbs := primaryResources(primary)
	name := fmt.Sprintf("%s-replica", util.DerefStr(primary.Name))

	reqBody := api.ReadReplicaSetCreate{
		Name:      name,
		Nodes:     1,
		CpuMillis: cpuMillis,
		MemoryGbs: memoryGbs,
	}

	statusOut := cmd.ErrOrStderr()
	fmt.Fprintf(statusOut, "🪞 Creating read replica '%s' (%d millicores CPU, %d GB memory)...\n", name, cpuMillis, memoryGbs)

	createCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.CreateReplicaSetWithResponse(createCtx, projectID, util.DerefStr(primary.ServiceId), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create read replica: %w", err)
	}

	if resp.StatusCode() != http.StatusAccepted || resp.JSON202 == nil {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	created := resp.JSON202
	fmt.Fprintf(statusOut, "✅ Read replica creation accepted (ID: %s)\n", util.DerefStr(created.Id))
	fmt.Fprintf(statusOut, "⏳ Waiting for read replica to become active (timeout: %v)...\n", replicaCreateWaitTimeout)

	ready, err := common.WaitForReplicaSet(ctx, common.WaitForReplicaSetArgs{
		Client:       client,
		ProjectID:    projectID,
		ServiceID:    util.DerefStr(primary.ServiceId),
		ReplicaSetID: util.DerefStr(created.Id),
		Output:       statusOut,
		Timeout:      replicaCreateWaitTimeout,
	})
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(statusOut, "🎉 Read replica is ready!\n")
	return ready, nil
}

// waitForEndpointReady polls host:port until it accepts TCP connections, or the
// timeout is reached.
func waitForEndpointReady(ctx context.Context, out io.Writer, host string, port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	// Fast path: skip the spinner if it's already reachable.
	if dialEndpoint(ctx, addr) {
		return nil
	}

	spinner := common.NewSpinner(out, "Waiting for read replica endpoint to accept connections...")
	defer spinner.Stop()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("read replica endpoint %s did not accept connections within %v", addr, timeout)
		case <-ticker.C:
			if dialEndpoint(ctx, addr) {
				return nil
			}
		}
	}
}

// dialEndpoint reports whether a TCP connection to addr can be established.
func dialEndpoint(ctx context.Context, addr string) bool {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// primaryResources returns the primary's CPU (millicores) and memory (GB) for a
// new replica to inherit, falling back to the smallest supported config.
func primaryResources(primary api.Service) (cpuMillis int, memoryGbs int) {
	cpuMillis, memoryGbs = 500, 2

	if primary.Resources == nil {
		return cpuMillis, memoryGbs
	}

	for _, r := range *primary.Resources {
		if r.Spec == nil {
			continue
		}
		if r.Spec.CpuMillis != nil && *r.Spec.CpuMillis > 0 {
			cpuMillis = *r.Spec.CpuMillis
		}
		if r.Spec.MemoryGbs != nil && *r.Spec.MemoryGbs > 0 {
			memoryGbs = *r.Spec.MemoryGbs
		}
	}

	return cpuMillis, memoryGbs
}

// connectTargetKind enumerates the choices in the connect target menu.
type connectTargetKind int

const (
	targetPrimary connectTargetKind = iota
	targetReplica
	targetCreate
	targetCancel
)

// connectTargetChoice is the result of the connect target menu.
type connectTargetChoice struct {
	kind    connectTargetKind
	replica *api.ReadReplicaSet
}

// connectTargetModel is the Bubble Tea model for selecting a connection target.
type connectTargetModel struct {
	options []string
	choices []connectTargetChoice
	cursor  int
	chosen  connectTargetChoice
}

func newConnectTargetModel(primary api.Service, replicas []api.ReadReplicaSet, allowCreate bool) connectTargetModel {
	options := []string{fmt.Sprintf("Connect to primary service (%s)", util.DerefStr(primary.ServiceId))}
	choices := []connectTargetChoice{{kind: targetPrimary}}

	for i := range replicas {
		options = append(options, fmt.Sprintf("Connect to read replica '%s'", util.DerefStr(replicas[i].Name)))
		choices = append(choices, connectTargetChoice{kind: targetReplica, replica: &replicas[i]})
	}

	// Only offer to create a new read replica when none already exist and
	// creation is permitted (i.e. not in read-only mode).
	if allowCreate && len(replicas) == 0 {
		options = append(options, "Create a new read replica and connect to it")
		choices = append(choices, connectTargetChoice{kind: targetCreate})
	}

	options = append(options, "Cancel")
	choices = append(choices, connectTargetChoice{kind: targetCancel})

	return connectTargetModel{
		options: options,
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
		switch msg.String() {
		case "ctrl+c", "q":
			m.chosen = connectTargetChoice{kind: targetCancel}
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.chosen = m.choices[m.cursor]
			return m, tea.Quit
		default:
			// Handle number keys based on available options
			if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
				idx := int(msg.String()[0] - '1') // '1' -> 0, '2' -> 1, etc.
				if idx >= 0 && idx < len(m.options) {
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

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, option)
	}

	s += "\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to cancel"
	return s
}

// selectConnectTargetOption shows the interactive menu for choosing a
// connection target.
func selectConnectTargetOption(out io.Writer, primary api.Service, replicas []api.ReadReplicaSet, allowCreate bool) (connectTargetChoice, error) {
	model := newConnectTargetModel(primary, replicas, allowCreate)

	program := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return connectTargetChoice{kind: targetCancel}, fmt.Errorf("failed to run connect menu: %w", err)
	}

	return finalModel.(connectTargetModel).chosen, nil
}
