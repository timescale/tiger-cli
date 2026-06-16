package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func hasChoice(m connectTargetModel, kind connectTargetKind) bool {
	for _, c := range m.choices {
		if c.kind == kind {
			return true
		}
	}
	return false
}

func TestNewConnectTargetModel_Options(t *testing.T) {
	// No replicas: primary, create, cancel.
	m := newConnectTargetModel(testPrimary(), nil, true)
	if len(m.options) != 3 {
		t.Fatalf("expected 3 options with no replicas, got %d: %v", len(m.options), m.options)
	}
	if m.choices[0].kind != targetPrimary {
		t.Errorf("expected first choice to be primary")
	}
	if m.choices[1].kind != targetCreate {
		t.Errorf("expected second choice to be create, got %v", m.choices[1].kind)
	}
	if m.choices[2].kind != targetCancel {
		t.Errorf("expected last choice to be cancel")
	}

	// Two replicas: primary, replica-a, replica-b, cancel (no create option).
	m = newConnectTargetModel(testPrimary(), testReplicas(), true)
	if len(m.options) != 4 {
		t.Fatalf("expected 4 options with two replicas, got %d: %v", len(m.options), m.options)
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
	if hasChoice(m, targetCreate) {
		t.Error("create option should not be offered when replicas exist")
	}

	// No replicas but creation disallowed (read-only): primary, cancel only.
	m = newConnectTargetModel(testPrimary(), nil, false)
	if hasChoice(m, targetCreate) {
		t.Error("create option should not be offered when creation is disallowed")
	}
	if len(m.options) != 2 {
		t.Fatalf("expected 2 options (primary, cancel) when create disallowed, got %d: %v", len(m.options), m.options)
	}
}

func TestConnectTargetModel_DefaultsToCancel(t *testing.T) {
	m := newConnectTargetModel(testPrimary(), testReplicas(), true)
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
			m := newConnectTargetModel(testPrimary(), testReplicas(), true)
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

func TestPrimaryResources(t *testing.T) {
	t.Run("inherits from primary", func(t *testing.T) {
		var primary api.Service
		if err := json.Unmarshal([]byte(`{"resources":[{"spec":{"cpu_millis":2000,"memory_gbs":8}}]}`), &primary); err != nil {
			t.Fatalf("failed to build service: %v", err)
		}
		cpu, mem := primaryResources(primary)
		if cpu != 2000 || mem != 8 {
			t.Errorf("expected 2000/8, got %d/%d", cpu, mem)
		}
	})

	t.Run("falls back when missing", func(t *testing.T) {
		cpu, mem := primaryResources(api.Service{})
		if cpu != 500 || mem != 2 {
			t.Errorf("expected fallback 500/2, got %d/%d", cpu, mem)
		}
	})
}

// listenTCP opens a loopback listener and returns its host, port, and the
// listener (so callers can keep it open or close it to make the port refused).
func listenTCP(t *testing.T) (host string, port int, ln net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port, ln
}

func TestWaitForEndpointReady_Listening(t *testing.T) {
	host, port, ln := listenTCP(t)
	defer ln.Close()

	if err := waitForEndpointReady(context.Background(), io.Discard, host, port, 5*time.Second); err != nil {
		t.Fatalf("expected endpoint to be ready, got: %v", err)
	}
}

func TestWaitForEndpointReady_BecomesReachable(t *testing.T) {
	// Close the port so the first dial is refused, then reopen it shortly after
	// so a later dial succeeds.
	host, port, ln := listenTCP(t)
	addr := ln.Addr().String()
	ln.Close()

	go func() {
		time.Sleep(3 * time.Second)
		if l, err := net.Listen("tcp", addr); err == nil {
			time.AfterFunc(10*time.Second, func() { l.Close() })
		}
	}()

	if err := waitForEndpointReady(context.Background(), io.Discard, host, port, 15*time.Second); err != nil {
		t.Fatalf("expected endpoint to become reachable, got: %v", err)
	}
}

func TestWaitForEndpointReady_Timeout(t *testing.T) {
	// Close the port immediately and never reopen it, so dials stay refused.
	host, port, ln := listenTCP(t)
	ln.Close()

	if err := waitForEndpointReady(context.Background(), io.Discard, host, port, 3*time.Second); err == nil {
		t.Fatal("expected timeout error for unreachable endpoint")
	}
}

func TestReplicaHasPooler(t *testing.T) {
	host := "h"
	port := 6432

	if replicaHasPooler(nil) {
		t.Error("nil replica should not have a pooler")
	}
	if replicaHasPooler(&api.ReadReplicaSet{}) {
		t.Error("replica without pooler should report false")
	}
	if replicaHasPooler(&api.ReadReplicaSet{ConnectionPooler: &api.ConnectionPooler{}}) {
		t.Error("pooler without endpoint should report false")
	}
	withPooler := &api.ReadReplicaSet{
		ConnectionPooler: &api.ConnectionPooler{Endpoint: &api.Endpoint{Host: &host, Port: &port}},
	}
	if !replicaHasPooler(withPooler) {
		t.Error("replica with pooler endpoint should report true")
	}
}

// TestResolveConnectTarget_ReadOnlyNoReplicasSkipsPrompt verifies that, in
// read-only mode with no replicas, resolveConnectTarget connects to the primary
// directly instead of showing a single-option menu (which would block on TTY
// input in this test).
func TestResolveConnectTarget_ReadOnlyNoReplicasSkipsPrompt(t *testing.T) {
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

	details, err := resolveConnectTarget(context.Background(), cmd, client, "proj-1", primary,
		common.ConnectionDetailsOptions{Role: "tsdbadmin"}, false /*noReplicaPrompt*/, true /*readOnly*/)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details == nil || details.Host != host {
		t.Fatalf("expected to connect directly to primary %q, got %+v", host, details)
	}
}
