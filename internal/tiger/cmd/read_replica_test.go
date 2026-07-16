package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

func testPrimary() api.Service {
	return api.Service{
		ServiceId: util.Ptr("svc-primary"),
		Name:      util.Ptr("my-db"),
	}
}

func testReplicas() []api.ReadReplicaSet {
	return []api.ReadReplicaSet{
		{Id: util.Ptr("rep-1"), Name: util.Ptr("replica-a")},
		{Id: util.Ptr("rep-2"), Name: util.Ptr("replica-b")},
	}
}

func TestNewConnectTargetModel_Options(t *testing.T) {
	// No replicas: primary, cancel.
	m := newConnectTargetModel(testPrimary(), nil)
	if len(m.choices) != 2 {
		t.Fatalf("expected 2 choices with no replicas, got %d: %v", len(m.choices), m.choices)
	}
	if m.choices[0].kind != targetPrimary {
		t.Errorf("expected first choice to be primary")
	}
	if m.choices[1].kind != targetCancel {
		t.Errorf("expected last choice to be cancel")
	}

	// Two replicas: primary, replica-a, replica-b, cancel.
	m = newConnectTargetModel(testPrimary(), testReplicas())
	if len(m.choices) != 4 {
		t.Fatalf("expected 4 choices with two replicas, got %d: %v", len(m.choices), m.choices)
	}
	if m.choices[1].kind != targetReplica || m.choices[1].replica == nil || *m.choices[1].replica.Id != "rep-1" {
		t.Errorf("expected second choice to be replica rep-1, got %+v", m.choices[1])
	}
	if m.choices[2].kind != targetReplica || *m.choices[2].replica.Id != "rep-2" {
		t.Errorf("expected third choice to be replica rep-2, got %+v", m.choices[2])
	}
	if m.choices[3].kind != targetCancel {
		t.Errorf("expected last choice to be cancel when replicas exist, got %v", m.choices[3].kind)
	}
}

func TestConnectTargetModel_DefaultsToCancel(t *testing.T) {
	m := newConnectTargetModel(testPrimary(), testReplicas())
	if m.chosen.kind != targetCancel {
		t.Errorf("expected default chosen to be cancel, got %v", m.chosen.kind)
	}
}

func TestConnectTargetModel_KeySelection(t *testing.T) {
	cases := []struct {
		name          string
		key           tea.KeyMsg
		wantKind      connectTargetKind
		wantReplicaID string // checked only when set
	}{
		{"q cancels", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, targetCancel, ""},
		{"enter selects primary (cursor starts at 0)", tea.KeyMsg{Type: tea.KeyEnter}, targetPrimary, ""},
		{"'2' selects the first replica", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, targetReplica, "rep-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newConnectTargetModel(testPrimary(), testReplicas())
			updated, _ := m.Update(tc.key)
			choice := updated.(connectTargetModel).chosen
			if choice.kind != tc.wantKind {
				t.Fatalf("expected kind %v, got %v", tc.wantKind, choice.kind)
			}
			if tc.wantReplicaID != "" && (choice.replica == nil || *choice.replica.Id != tc.wantReplicaID) {
				t.Errorf("expected replica %s, got %+v", tc.wantReplicaID, choice.replica)
			}
		})
	}
}

// TestSelectConnection_NoReplicasSkipsPrompt verifies that, with no
// connectable replicas, selectConnection connects to the primary directly
// instead of showing a single-option menu (which would block on TTY input in
// this test).
func TestSelectConnection_NoReplicasSkipsPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) // no replicas
	}))
	defer server.Close()

	client, err := api.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	host := "primary.example.com"
	port := 5432
	primary := api.Service{
		ServiceId: util.Ptr("svc-primary"),
		Name:      util.Ptr("my-db"),
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}

	// Pretend we're on a TTY so the prompt would normally run.
	orig := checkStdinIsTTY
	checkStdinIsTTY = func() bool { return true }
	defer func() { checkStdinIsTTY = orig }()

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	target := &common.ConnectionTarget{ConnectionService: primary, CredentialService: primary}
	details, err := selectConnection(context.Background(), cmd, client, "proj-1", target,
		common.ConnectionDetailsOptions{Role: "tsdbadmin"}, false /*noReplicaPrompt*/)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details == nil || details.Host != host {
		t.Fatalf("expected to connect directly to primary %q, got %+v", host, details)
	}
}
