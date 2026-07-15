package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// replicaConnectTestConfig builds a Config whose client is backed by an
// httptest server that 404s on getService (so resolution falls back to the
// scan) and returns the given services from the list endpoint.
func replicaConnectTestConfig(t *testing.T, list []api.Service) *common.Config {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/services") {
			_ = json.NewEncoder(w).Encode(list)
			return
		}
		// getService for a replica ID is a miss.
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "service not found"})
	}))
	t.Cleanup(srv.Close)

	client, err := api.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}
	return &common.Config{Config: &config.Config{}, ProjectID: "proj1", Client: client}
}

func replicaConnectTestService() api.Service {
	rhost := "replica.example.com"
	rport := 5432
	return api.Service{
		ServiceId: util.Ptr("svcprimary"),
		ProjectId: util.Ptr("proj1"),
		Name:      util.Ptr("my-db"),
		ReadReplicaSets: &[]api.ReadReplicaSet{{
			Id:       util.Ptr("rep1234567"),
			Name:     util.Ptr("reporting-replica"),
			Status:   util.Ptr(api.ReadReplicaSetStatusActive),
			Endpoint: &api.Endpoint{Host: &rhost, Port: &rport},
		}},
	}
}

// TestResolveConnectionTarget_Primary: when the service lookup succeeds, the ID
// is a primary and no replica is returned.
func TestResolveConnectionTarget_Primary(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return api.Service{ServiceId: util.Ptr("svcprimary")}, nil
	}
	defer func() { getServiceDetailsFunc = orig }()

	cfg := &common.Config{Config: &config.Config{}, ProjectID: "proj1"}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	service, replica, err := resolveConnectionTarget(cmd, cfg, []string{"svcprimary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replica != nil {
		t.Fatalf("expected no replica for a primary ID, got %+v", replica)
	}
	if util.DerefStr(service.ServiceId) != "svcprimary" {
		t.Errorf("expected primary svcprimary, got %q", util.DerefStr(service.ServiceId))
	}
}

// TestResolveConnectionTarget_ReplicaFallback: when the service lookup fails,
// the ID is scanned against the project's read replicas, and the matching
// replica plus its parent service is returned.
func TestResolveConnectionTarget_ReplicaFallback(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return api.Service{}, fmt.Errorf("service not found")
	}
	defer func() { getServiceDetailsFunc = orig }()

	cfg := replicaConnectTestConfig(t, []api.Service{replicaConnectTestService()})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	service, replica, err := resolveConnectionTarget(cmd, cfg, []string{"rep1234567"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replica == nil {
		t.Fatal("expected a replica to be resolved, got nil")
	}
	if util.DerefStr(replica.Id) != "rep1234567" {
		t.Errorf("expected replica rep1234567, got %q", util.DerefStr(replica.Id))
	}
	// Parent service (used for credentials) is returned alongside the replica.
	if util.DerefStr(service.ServiceId) != "svcprimary" {
		t.Errorf("expected parent svcprimary, got %q", util.DerefStr(service.ServiceId))
	}
}

// TestResolveConnectionTarget_UnknownReturnsOriginalError: an ID that is neither
// a service nor a replica surfaces the original service-lookup error.
func TestResolveConnectionTarget_UnknownReturnsOriginalError(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return api.Service{}, fmt.Errorf("original lookup error")
	}
	defer func() { getServiceDetailsFunc = orig }()

	cfg := replicaConnectTestConfig(t, []api.Service{replicaConnectTestService()})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	_, _, err := resolveConnectionTarget(cmd, cfg, []string{"missing999"})
	if err == nil {
		t.Fatal("expected an error for an unknown ID, got nil")
	}
	if !strings.Contains(err.Error(), "original lookup error") {
		t.Errorf("expected the original lookup error to be surfaced, got %v", err)
	}
}

// TestBuildConnectionDetailsForTarget_ReplicaPoolerFallback: requesting --pooled
// on a replica with no pooler warns and falls back to a direct connection.
func TestBuildConnectionDetailsForTarget_ReplicaPoolerFallback(t *testing.T) {
	rhost := "replica.example.com"
	rport := 5432
	replica := &api.ReadReplicaSet{
		Id:       util.Ptr("rep1234567"),
		Name:     util.Ptr("reporting-replica"),
		Endpoint: &api.Endpoint{Host: &rhost, Port: &rport},
	}
	primary := api.Service{ServiceId: util.Ptr("svcprimary"), ProjectId: util.Ptr("proj1")}

	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetErr(buf)
	cmd.SetOut(io.Discard)

	details, err := buildConnectionDetailsForTarget(cmd, primary, replica, common.ConnectionDetailsOptions{
		Pooled: true,
		Role:   "tsdbadmin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details.Host != rhost {
		t.Errorf("expected replica host %q, got %q", rhost, details.Host)
	}
	if details.IsPooler {
		t.Error("expected fallback to a direct (non-pooler) connection")
	}
	if !strings.Contains(buf.String(), "no connection pooler") {
		t.Errorf("expected a pooler-fallback warning, got %q", buf.String())
	}
}

// TestBuildConnectionDetailsForTarget_PrimaryRequiresPooler: requesting --pooled
// on a primary with no pooler is a hard error.
func TestBuildConnectionDetailsForTarget_PrimaryRequiresPooler(t *testing.T) {
	host := "primary.example.com"
	port := 5432
	primary := api.Service{
		ServiceId: util.Ptr("svcprimary"),
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}

	cmd := &cobra.Command{}
	cmd.SetErr(io.Discard)
	cmd.SetOut(io.Discard)

	_, err := buildConnectionDetailsForTarget(cmd, primary, nil, common.ConnectionDetailsOptions{
		Pooled: true,
		Role:   "tsdbadmin",
	})
	if err == nil {
		t.Fatal("expected an error when a pooler is unavailable for the primary, got nil")
	}
}
