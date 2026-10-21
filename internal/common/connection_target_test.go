package common

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
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

// replicaSet returns an active read replica set of primaryService.
func replicaSet() api.ReadReplicaSet {
	host := "replica.example.com"
	port := 5432
	return api.ReadReplicaSet{
		ID:        "rep1234567",
		ServiceID: "svcprimary",
		Name:      "reporting-replica",
		Status:    api.ReadReplicaSetStatusActive,
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}
}

// replicaService returns a standby read replica of primaryService, shaped
// the way GetService returns it when given a read replica set ID.
func replicaService() api.Service {
	host := "replica.example.com"
	port := 5432
	return api.Service{
		ServiceID: "rep1234567",
		ProjectID: "proj1",
		Name:      "reporting-replica",
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
		ForkedFrom: &api.ForkSpec{
			IsStandby: new(true),
			ProjectID: new("proj1"),
			ServiceID: new("svcprimary"),
		},
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
	if target.Kind != TargetService || target.ID != "svcprimary" || target.Credentials.ID != "svcprimary" {
		t.Errorf("expected primary target for a primary ID, got %+v", target)
	}

	// Replica ID → replica target with parent credentials.
	target, err = ResolveConnectionTargetByID(context.Background(), client, "proj1", "rep1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Kind != TargetReadReplica || target.ID != "rep1234567" || target.Credentials.ID != "svcprimary" {
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

	target := NewReplicaConnectionTarget(replica, ServiceCredentials(primary))
	if target.Kind != TargetReadReplica {
		t.Error("expected a replica target")
	}
	if target.ID != "rep7654321" || util.DerefStr(target.Endpoint.Host) != host {
		t.Errorf("expected connect to carry the replica endpoint, got %+v", target)
	}
	if target.Credentials.ID != "svcprimary" || target.Credentials.Name != "my-db" {
		t.Errorf("expected credential to be the primary, got %+v", target.Credentials)
	}
}

// TestConnectionTargetReadOnlyVerdict checks that a replica is judged on its own
// environment tag rather than its primary's, which requires that tag to survive
// NewReplicaConnectionTarget.
func TestConnectionTargetReadOnlyVerdict(t *testing.T) {
	tagged := func(svc api.Service, tag string) api.Service {
		if tag != "" {
			svc.Metadata = &api.ServiceMetadata{Environment: new(tag)}
		}
		return svc
	}

	cases := []struct {
		name        string
		mode        config.ReadOnlyMode
		primaryTag  string
		replicaTag  string
		wantSession bool
	}{
		{name: "off never forces read-only", mode: config.ReadOnlyOff, primaryTag: "PROD", replicaTag: "PROD"},
		{name: "all always forces read-only", mode: config.ReadOnlyAll, wantSession: true},
		{name: "prod leaves a DEV replica writable", mode: config.ReadOnlyProd, primaryTag: "DEV", replicaTag: "DEV"},
		{name: "prod protects a PROD replica", mode: config.ReadOnlyProd, primaryTag: "DEV", replicaTag: "PROD", wantSession: true},
		{name: "prod ignores the primary's tag", mode: config.ReadOnlyProd, primaryTag: "PROD", replicaTag: "DEV"},
		{name: "prod leaves an untagged replica writable", mode: config.ReadOnlyProd, primaryTag: "PROD"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			primary := tagged(primaryService(), tt.primaryTag)
			replica := api.ReadReplicaSet{ID: "rep1", Name: "rep"}
			if tt.replicaTag != "" {
				replica.Metadata = &api.ReplicaSetMetadata{Environment: new(tt.replicaTag)}
			}

			target := NewReplicaConnectionTarget(replica, ServiceCredentials(primary))
			cfg := &config.Config{ReadOnly: tt.mode}
			got := CheckReadOnly(cfg, target.Environment) != nil
			if got != tt.wantSession {
				t.Errorf("CheckReadOnly(read_only=%q, primary=%q, replica=%q) != nil = %t, want %t",
					tt.mode, tt.primaryTag, tt.replicaTag, got, tt.wantSession)
			}
		})
	}
}

// TestConnectionTargetDetails covers Details, the exported way onto the
// endpoint/credential split: which endpoint it selects, and the pooler policy
// that deliberately differs between a primary and a replica.
func TestConnectionTargetDetails(t *testing.T) {
	pooler := &api.ConnectionPooler{
		Endpoint: &api.Endpoint{
			Host: new("replica-pooler.example.com"),
			Port: new(6432),
		},
	}

	// The replica supplies the endpoint, the primary the credentials — which
	// aren't exercised here, since WithPassword is off.
	replicaTarget := func(overrides ...func(*api.ReadReplicaSet)) *ConnectionTarget {
		replica := replicaSet()
		for _, override := range overrides {
			override(&replica)
		}
		return NewReplicaConnectionTarget(replica, ServiceCredentials(primaryService()))
	}

	cases := []struct {
		name    string
		target  *ConnectionTarget
		pooled  bool
		want    *ConnectionDetails
		wantErr string
	}{
		{
			name:   "replica connects to its own direct endpoint",
			target: replicaTarget(),
			want: &ConnectionDetails{
				Role:     "tsdbadmin",
				Host:     "replica.example.com",
				Port:     5432,
				Database: "tsdb",
			},
		},
		{
			name: "replica uses its pooler when one is available",
			target: replicaTarget(func(r *api.ReadReplicaSet) {
				r.ConnectionPooler = pooler
			}),
			pooled: true,
			want: &ConnectionDetails{
				Role:     "tsdbadmin",
				Host:     "replica-pooler.example.com",
				Port:     6432,
				Database: "tsdb",
				IsPooler: true,
			},
		},
		{
			// A replica with no pooler falls back rather than failing; the
			// caller surfaces that via ReplicaPoolerWarning.
			name:   "replica falls back to direct when its pooler is missing",
			target: replicaTarget(),
			pooled: true,
			want: &ConnectionDetails{
				Role:     "tsdbadmin",
				Host:     "replica.example.com",
				Port:     5432,
				Database: "tsdb",
			},
		},
		{
			// For a primary the same request is a hard error instead.
			name:    "primary refuses when its pooler is missing",
			target:  NewServiceConnectionTarget(primaryService()),
			pooled:  true,
			wantErr: "connection pooler not available for this service",
		},
		{
			name: "missing endpoint is reported",
			target: replicaTarget(func(r *api.ReadReplicaSet) {
				r.Endpoint = nil
			}),
			wantErr: "failed to build connection string: service endpoint not available",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			details, err := tc.target.Details(testConfig(""), ConnectionDetailsOptions{
				Role:   "tsdbadmin",
				Pooled: tc.pooled,
			})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Errorf("error = %q, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// AllowUnexported covers readOnly, which no case here sets.
			if diff := cmp.Diff(tc.want, details, cmp.AllowUnexported(ConnectionDetails{})); diff != "" {
				t.Errorf("Details() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReplicaPoolerWarning(t *testing.T) {
	host := "h"
	port := 6432
	withPooler := api.ReadReplicaSet{
		Name:             "rep-a",
		ConnectionPooler: &api.ConnectionPooler{Endpoint: &api.Endpoint{Host: &host, Port: &port}},
	}
	noPooler := api.ReadReplicaSet{Name: "rep-b"}

	replica := func(replica api.ReadReplicaSet) *ConnectionTarget {
		return NewReplicaConnectionTarget(replica, ServiceCredentials(primaryService()))
	}

	cases := []struct {
		name        string
		target      *ConnectionTarget
		pooled      bool
		wantWarning bool
	}{
		{"primary is never warned", NewServiceConnectionTarget(primaryService()), true, false},
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

func TestResolveConnectionTargetByID_ReplicaParentFetchFails(t *testing.T) {
	// The replica itself resolves, but its parent is absent from the server.
	client := serviceTestClient(t, map[string]api.Service{"rep1234567": replicaService()})

	if _, err := ResolveConnectionTargetByID(context.Background(), client, "proj1", "rep1234567"); err == nil {
		t.Fatal("expected an error when the parent service can't be fetched, got nil")
	}
}

func TestResolveConnectionTargetByID_ReplicaWithoutParentID(t *testing.T) {
	// A standby with no parent recorded has nowhere else to look for
	// credentials, so it stands in as its own credential owner.
	replica := replicaService()
	replica.ForkedFrom.ServiceID = nil
	client := serviceTestClient(t, map[string]api.Service{"rep1234567": replica})

	target, err := ResolveConnectionTargetByID(context.Background(), client, "proj1", "rep1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Kind != TargetReadReplica {
		t.Error("expected a replica target")
	}
	if target.Credentials.ID != "rep1234567" {
		t.Errorf("expected the replica as its own credential owner, got %q", target.Credentials.ID)
	}
}
