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

// serviceClientConfig builds a Config whose client serves the getService
// endpoint from the given services keyed by ID (404 when absent).
func serviceClientConfig(t *testing.T, services map[string]api.Service) *common.Config {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		svc, ok := services[parts[len(parts)-1]]
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
	return &common.Config{Config: &config.Config{}, ProjectID: "proj1", Client: client}
}

func primarySvc() api.Service {
	host := "svcprimary.example.com"
	port := 5432
	return api.Service{
		ServiceId: util.Ptr("svcprimary"),
		ProjectId: util.Ptr("proj1"),
		Name:      util.Ptr("my-db"),
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}
}

func standbySvc() api.Service {
	host := "replica.example.com"
	port := 5432
	return api.Service{
		ServiceId: util.Ptr("rep1234567"),
		ProjectId: util.Ptr("proj1"),
		Name:      util.Ptr("reporting-replica"),
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
		ForkedFrom: &api.ForkSpec{
			IsStandby: util.Ptr(true),
			ProjectId: util.Ptr("proj1"),
			ServiceId: util.Ptr("svcprimary"),
		},
	}
}

// TestResolveConnectionTarget_Primary: a plain service resolves to a primary
// target (connect == credential, no parent fetch, so no client needed).
func TestResolveConnectionTarget_Primary(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return primarySvc(), nil
	}
	defer func() { getServiceDetailsFunc = orig }()

	cfg := &common.Config{Config: &config.Config{}, ProjectID: "proj1"}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	target, err := resolveConnectionTarget(cmd, cfg, []string{"svcprimary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.IsReplica {
		t.Fatal("expected a primary target")
	}
	if util.DerefStr(target.Connect.ServiceId) != "svcprimary" {
		t.Errorf("expected connect svcprimary, got %q", util.DerefStr(target.Connect.ServiceId))
	}
}

// TestResolveConnectionTarget_Replica: a standby service connects to the replica
// but resolves credentials against the parent (fetched via the client).
func TestResolveConnectionTarget_Replica(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return standbySvc(), nil
	}
	defer func() { getServiceDetailsFunc = orig }()

	cfg := serviceClientConfig(t, map[string]api.Service{"svcprimary": primarySvc()})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	target, err := resolveConnectionTarget(cmd, cfg, []string{"rep1234567"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !target.IsReplica {
		t.Fatal("expected a replica target")
	}
	if util.DerefStr(target.Connect.ServiceId) != "rep1234567" {
		t.Errorf("expected connect rep1234567, got %q", util.DerefStr(target.Connect.ServiceId))
	}
	if util.DerefStr(target.Credential.ServiceId) != "svcprimary" {
		t.Errorf("expected credential svcprimary, got %q", util.DerefStr(target.Credential.ServiceId))
	}
}

// TestResolveConnectionTarget_LookupError: a service-lookup failure is surfaced.
func TestResolveConnectionTarget_LookupError(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, cfg *common.Config, args []string) (api.Service, error) {
		return api.Service{}, fmt.Errorf("lookup failed")
	}
	defer func() { getServiceDetailsFunc = orig }()

	cfg := &common.Config{Config: &config.Config{}, ProjectID: "proj1"}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if _, err := resolveConnectionTarget(cmd, cfg, []string{"x"}); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestBuildConnectionDetailsForTarget_ReplicaPoolerFallback: requesting --pooled
// on a replica with no pooler warns and falls back to a direct connection.
func TestBuildConnectionDetailsForTarget_ReplicaPoolerFallback(t *testing.T) {
	rhost := "replica.example.com"
	rport := 5432
	target := &common.ConnectionTarget{
		Connect: api.Service{
			ServiceId: util.Ptr("rep1234567"),
			Name:      util.Ptr("reporting-replica"),
			Endpoint:  &api.Endpoint{Host: &rhost, Port: &rport},
		},
		Credential: api.Service{ServiceId: util.Ptr("svcprimary"), ProjectId: util.Ptr("proj1")},
		IsReplica:  true,
	}

	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetErr(buf)
	cmd.SetOut(io.Discard)

	details, err := buildConnectionDetailsForTarget(cmd, target, common.ConnectionDetailsOptions{Pooled: true, Role: "tsdbadmin"})
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
	svc := api.Service{
		ServiceId: util.Ptr("svcprimary"),
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}
	target := &common.ConnectionTarget{Connect: svc, Credential: svc}

	cmd := &cobra.Command{}
	cmd.SetErr(io.Discard)
	cmd.SetOut(io.Discard)

	if _, err := buildConnectionDetailsForTarget(cmd, target, common.ConnectionDetailsOptions{Pooled: true, Role: "tsdbadmin"}); err == nil {
		t.Fatal("expected an error when a pooler is unavailable for the primary, got nil")
	}
}
