package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// replicaTestClient builds an API client backed by an httptest server that
// serves the getService endpoint from services (keyed by service ID, 404 when
// absent) and the getServices list endpoint from list.
func replicaTestClient(t *testing.T, services map[string]api.Service, list []api.Service) *api.ClientWithResponses {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// List: /projects/{project_id}/services
		if strings.HasSuffix(r.URL.Path, "/services") {
			_ = json.NewEncoder(w).Encode(list)
			return
		}

		// Get: /projects/{project_id}/services/{service_id}
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

func testReplicaService() api.Service {
	rhost := "replica.example.com"
	rport := 5432
	return api.Service{
		ServiceId: util.Ptr("svcprimary"),
		ProjectId: util.Ptr("proj1"),
		Name:      util.Ptr("my-db"),
		ReadReplicaSets: &[]api.ReadReplicaSet{
			{
				Id:       util.Ptr("rep1234567"),
				Name:     util.Ptr("reporting-replica"),
				Status:   util.Ptr(api.ReadReplicaSetStatusActive),
				Endpoint: &api.Endpoint{Host: &rhost, Port: &rport},
			},
		},
	}
}

func TestResolveServiceOrReplica_Primary(t *testing.T) {
	primary := testReplicaService()
	client := replicaTestClient(t, map[string]api.Service{"svcprimary": primary}, []api.Service{primary})

	target, err := ResolveServiceOrReplica(context.Background(), client, "proj1", "svcprimary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Replica != nil {
		t.Fatalf("expected a primary target, got a replica: %+v", target.Replica)
	}
	if util.DerefStr(target.Service.ServiceId) != "svcprimary" {
		t.Errorf("expected primary svcprimary, got %q", util.DerefStr(target.Service.ServiceId))
	}
}

func TestResolveServiceOrReplica_Replica(t *testing.T) {
	primary := testReplicaService()
	// The replica ID is not addressable as a service, so getService 404s and
	// resolution must fall back to scanning the services list.
	client := replicaTestClient(t, map[string]api.Service{"svcprimary": primary}, []api.Service{primary})

	target, err := ResolveServiceOrReplica(context.Background(), client, "proj1", "rep1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Replica == nil {
		t.Fatalf("expected a replica target, got primary only")
	}
	// Parent service is used for credentials.
	if util.DerefStr(target.Service.ServiceId) != "svcprimary" {
		t.Errorf("expected parent svcprimary, got %q", util.DerefStr(target.Service.ServiceId))
	}
	// Replica carries the endpoint we should connect to.
	if util.DerefStr(target.Replica.Id) != "rep1234567" {
		t.Errorf("expected replica rep1234567, got %q", util.DerefStr(target.Replica.Id))
	}
	if target.Replica.Endpoint == nil || util.DerefStr(target.Replica.Endpoint.Host) != "replica.example.com" {
		t.Errorf("expected replica endpoint host, got %+v", target.Replica.Endpoint)
	}
}

func TestResolveServiceOrReplica_NotFound(t *testing.T) {
	primary := testReplicaService()
	client := replicaTestClient(t, map[string]api.Service{"svcprimary": primary}, []api.Service{primary})

	// An ID that is neither a service nor a known replica surfaces an error.
	_, err := ResolveServiceOrReplica(context.Background(), client, "proj1", "unknown999")
	if err == nil {
		t.Fatal("expected an error for an unknown ID, got nil")
	}
}

func TestReplicaPoolerWarning(t *testing.T) {
	host := "h"
	port := 6432
	withPooler := &api.ReadReplicaSet{
		Name:             util.Ptr("rep-a"),
		ConnectionPooler: &api.ConnectionPooler{Endpoint: &api.Endpoint{Host: &host, Port: &port}},
	}
	noPooler := &api.ReadReplicaSet{Name: util.Ptr("rep-b")}

	cases := []struct {
		name        string
		replica     *api.ReadReplicaSet
		pooled      bool
		wantWarning bool
	}{
		{"nil replica", nil, true, false},
		{"not pooled, no pooler", noPooler, false, false},
		{"not pooled, has pooler", withPooler, false, false},
		{"pooled, has pooler", withPooler, true, false},
		{"pooled, no pooler warns", noPooler, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warning := ReplicaPoolerWarning(tc.replica, tc.pooled)
			if (warning != "") != tc.wantWarning {
				t.Errorf("warning = %q, wantWarning = %v", warning, tc.wantWarning)
			}
		})
	}
}

func TestFindReplicaByID(t *testing.T) {
	primary := testReplicaService()
	client := replicaTestClient(t, map[string]api.Service{"svcprimary": primary}, []api.Service{primary})

	target, err := FindReplicaByID(context.Background(), client, "proj1", "rep1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Replica == nil || util.DerefStr(target.Replica.Id) != "rep1234567" {
		t.Errorf("expected to find replica rep1234567, got %+v", target)
	}

	_, err = FindReplicaByID(context.Background(), client, "proj1", "missing999")
	if !errors.Is(err, ErrReplicaNotFound) {
		t.Errorf("expected ErrReplicaNotFound when no replica matches, got %v", err)
	}
}

// TestFindReplicaByID_NotActive: a matched replica that isn't active returns a
// descriptive error (not ErrReplicaNotFound), so callers surface it.
func TestFindReplicaByID_NotActive(t *testing.T) {
	primary := api.Service{
		ServiceId: util.Ptr("svcprimary"),
		ProjectId: util.Ptr("proj1"),
		ReadReplicaSets: &[]api.ReadReplicaSet{{
			Id:     util.Ptr("rep1234567"),
			Name:   util.Ptr("still-creating"),
			Status: util.Ptr(api.ReadReplicaSetStatusCreating),
		}},
	}
	client := replicaTestClient(t, map[string]api.Service{"svcprimary": primary}, []api.Service{primary})

	_, err := FindReplicaByID(context.Background(), client, "proj1", "rep1234567")
	if err == nil {
		t.Fatal("expected an error for a non-active replica, got nil")
	}
	if errors.Is(err, ErrReplicaNotFound) {
		t.Errorf("expected a descriptive not-active error, got ErrReplicaNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Errorf("expected a 'not active' error, got %v", err)
	}
}

// TestResolveServiceOrReplica_NonNotFoundSkipsScan: a non-404 service lookup
// error is surfaced without listing services to look for a replica.
func TestResolveServiceOrReplica_NonNotFoundSkipsScan(t *testing.T) {
	primary := testReplicaService()
	listed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/services") {
			listed = true
			_ = json.NewEncoder(w).Encode([]api.Service{primary})
			return
		}
		// GET service → permission denied (not a 404).
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "forbidden"})
	}))
	t.Cleanup(srv.Close)

	client, err := api.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	// "rep1234567" would match a replica if scanned, but a 403 must skip the scan.
	if _, err := ResolveServiceOrReplica(context.Background(), client, "proj1", "rep1234567"); err == nil {
		t.Fatal("expected the 403 error to be surfaced, got nil")
	}
	if listed {
		t.Error("expected no service-list scan for a non-404 service error")
	}
}
