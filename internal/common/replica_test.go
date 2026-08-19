package common

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/util"
)

// serviceTestClient builds an API client backed by an httptest server that
// serves the getService endpoint from services keyed by service ID (404 when
// absent).
func serviceTestClient(t *testing.T, services map[string]api.Service) *api.ClientWithResponses {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		id := parts[len(parts)-1]
		svc, ok := services[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "service not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(svc)
	}))
	t.Cleanup(srv.Close)

	client, err := api.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return client
}

func primaryService() api.Service {
	host := "svcprimary.example.com"
	port := 5432
	return api.Service{
		ServiceID: "svcprimary",
		ProjectID: "proj1",
		Name:      "my-db",
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}
}

func replicaService() api.Service {
	host := "replica.example.com"
	port := 5432
	return api.Service{
		ServiceID: "rep1234567",
		ProjectID: "proj1",
		Name:      "reporting-replica",
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
		ForkedFrom: &api.ForkSpec{
			IsStandby: util.Ptr(true),
			ProjectID: util.Ptr("proj1"),
			ServiceID: util.Ptr("svcprimary"),
		},
	}
}

func TestResolveConnectionTarget_Primary(t *testing.T) {
	primary := primaryService()
	client := serviceTestClient(t, map[string]api.Service{"svcprimary": primary})

	target, err := ResolveConnectionTarget(context.Background(), client, "proj1", primary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.IsReplica {
		t.Fatal("expected a primary target")
	}
	if target.ConnectionService.ServiceID != "svcprimary" || target.CredentialService.ServiceID != "svcprimary" {
		t.Errorf("expected connect and credential to be the primary, got %+v", target)
	}
}

func TestResolveConnectionTarget_Replica(t *testing.T) {
	primary := primaryService()
	replica := replicaService()
	client := serviceTestClient(t, map[string]api.Service{"svcprimary": primary})

	target, err := ResolveConnectionTarget(context.Background(), client, "proj1", replica)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !target.IsReplica {
		t.Fatal("expected a replica target")
	}
	// Connect to the replica's own endpoint.
	if target.ConnectionService.ServiceID != "rep1234567" {
		t.Errorf("expected connect service rep1234567, got %q", target.ConnectionService.ServiceID)
	}
	// Credentials resolve against the parent primary (fetched via GetService).
	if target.CredentialService.ServiceID != "svcprimary" {
		t.Errorf("expected credential service svcprimary, got %q", target.CredentialService.ServiceID)
	}
}

func TestResolveConnectionTarget_ReplicaParentFetchFails(t *testing.T) {
	replica := replicaService()
	// Parent svcprimary is absent from the server → parent fetch 404s.
	client := serviceTestClient(t, map[string]api.Service{})

	if _, err := ResolveConnectionTarget(context.Background(), client, "proj1", replica); err == nil {
		t.Fatal("expected an error when the parent service can't be fetched, got nil")
	}
}

func TestResolveConnectionTargetByID(t *testing.T) {
	primary := primaryService()
	replica := replicaService()
	client := serviceTestClient(t, map[string]api.Service{"svcprimary": primary, "rep1234567": replica})

	// Primary ID → primary target.
	target, err := ResolveConnectionTargetByID(context.Background(), client, "proj1", "svcprimary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.IsReplica {
		t.Error("expected primary target for a primary ID")
	}

	// Replica ID → replica target with parent credentials.
	target, err = ResolveConnectionTargetByID(context.Background(), client, "proj1", "rep1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !target.IsReplica || target.CredentialService.ServiceID != "svcprimary" {
		t.Errorf("expected replica target with parent credentials, got %+v", target)
	}

	// Unknown ID → error.
	if _, err := ResolveConnectionTargetByID(context.Background(), client, "proj1", "missing9999"); err == nil {
		t.Error("expected an error for an unknown ID, got nil")
	}
}

func TestNewReplicaConnectionTarget(t *testing.T) {
	primary := primaryService()
	host := "menu-replica.example.com"
	port := 5432
	replica := api.ReadReplicaSet{
		ID:       "rep7654321",
		Name:     "menu-replica",
		Endpoint: &api.Endpoint{Host: &host, Port: &port},
	}

	target := NewReplicaConnectionTarget(primary, replica)
	if !target.IsReplica {
		t.Error("expected a replica target")
	}
	if target.ConnectionService.ServiceID != "rep7654321" || util.DerefStr(target.ConnectionService.Endpoint.Host) != host {
		t.Errorf("expected connect to carry the replica endpoint, got %+v", target.ConnectionService)
	}
	if target.CredentialService.ServiceID != "svcprimary" {
		t.Errorf("expected credential to be the primary, got %q", target.CredentialService.ServiceID)
	}
}

func TestReplicaPoolerWarning(t *testing.T) {
	host := "h"
	port := 6432
	withPooler := api.Service{
		Name:             "rep-a",
		ConnectionPooler: &api.ConnectionPooler{Endpoint: &api.Endpoint{Host: &host, Port: &port}},
	}
	noPooler := api.Service{Name: "rep-b"}

	replica := func(svc api.Service) *ConnectionTarget {
		return &ConnectionTarget{ConnectionService: svc, CredentialService: primaryService(), IsReplica: true}
	}

	cases := []struct {
		name        string
		target      *ConnectionTarget
		pooled      bool
		wantWarning bool
	}{
		{"primary is never warned", &ConnectionTarget{ConnectionService: noPooler, CredentialService: noPooler}, true, false},
		{"replica not pooled, no pooler", replica(noPooler), false, false},
		{"replica not pooled, has pooler", replica(withPooler), false, false},
		{"replica pooled, has pooler", replica(withPooler), true, false},
		{"replica pooled, no pooler warns", replica(noPooler), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warning := ReplicaPoolerWarning(tc.target, tc.pooled)
			if (warning != "") != tc.wantWarning {
				t.Errorf("warning = %q, wantWarning = %v", warning, tc.wantWarning)
			}
		})
	}
}
