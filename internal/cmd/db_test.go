package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func setupDBTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Disable analytics for DB tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	t.Cleanup(func() {
		// Clean up environment variables BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeDBCommand(ctx context.Context, args ...string) (string, error) {
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err
}

// serviceClientApp builds an App whose client serves the getService endpoint
// from the given services keyed by ID (404 when absent).
func serviceClientApp(t *testing.T, services map[string]api.Service) *common.App {
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
	return newTestApp(t, client, "proj1")
}

func primarySvc() api.Service {
	host := "svcprimary.example.com"
	port := 5432
	return api.Service{
		ServiceID: "svcprimary",
		ProjectID: "proj1",
		Name:      "my-db",
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}
}

func standbySvc() api.Service {
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

// TestLookupConnectionTarget_Primary: a plain service resolves to a primary
// target (connect == credential, no parent fetch, so no client needed).
func TestLookupConnectionTarget_Primary(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return primarySvc(), nil
	}
	defer func() { getServiceDetailsFunc = orig }()

	app := newTestApp(t, nil, "proj1")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	target, err := lookupConnectionTarget(cmd, app, []string{"svcprimary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.IsReplica {
		t.Fatal("expected a primary target")
	}
	if target.ConnectionService.ServiceID != "svcprimary" {
		t.Errorf("expected connect svcprimary, got %q", target.ConnectionService.ServiceID)
	}
}

// TestLookupConnectionTarget_Replica: a standby service connects to the replica
// but resolves credentials against the parent (fetched via the client).
func TestLookupConnectionTarget_Replica(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return standbySvc(), nil
	}
	defer func() { getServiceDetailsFunc = orig }()

	app := serviceClientApp(t, map[string]api.Service{"svcprimary": primarySvc()})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	target, err := lookupConnectionTarget(cmd, app, []string{"rep1234567"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !target.IsReplica {
		t.Fatal("expected a replica target")
	}
	if target.ConnectionService.ServiceID != "rep1234567" {
		t.Errorf("expected connect rep1234567, got %q", target.ConnectionService.ServiceID)
	}
	if target.CredentialService.ServiceID != "svcprimary" {
		t.Errorf("expected credential svcprimary, got %q", target.CredentialService.ServiceID)
	}
}

// TestLookupConnectionTarget_LookupError: a service-lookup failure is surfaced.
func TestLookupConnectionTarget_LookupError(t *testing.T) {
	orig := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return api.Service{}, fmt.Errorf("lookup failed")
	}
	defer func() { getServiceDetailsFunc = orig }()

	app := newTestApp(t, nil, "proj1")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if _, err := lookupConnectionTarget(cmd, app, []string{"x"}); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestBuildConnectionDetailsForTarget_ReplicaPoolerFallback: requesting --pooled
// on a replica with no pooler warns and falls back to a direct connection.
func TestBuildConnectionDetailsForTarget_ReplicaPoolerFallback(t *testing.T) {
	rhost := "replica.example.com"
	rport := 5432
	target := &common.ConnectionTarget{
		ConnectionService: api.Service{
			ServiceID: "rep1234567",
			Name:      "reporting-replica",
			Endpoint:  &api.Endpoint{Host: &rhost, Port: &rport},
		},
		CredentialService: api.Service{ServiceID: "svcprimary", ProjectID: "proj1"},
		IsReplica:         true,
	}

	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetErr(buf)
	cmd.SetOut(io.Discard)

	details, err := buildConnectionDetailsForTarget(cmd, testConfig(t), target, common.ConnectionDetailsOptions{Pooled: true, Role: "tsdbadmin"})
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
		ServiceID: "svcprimary",
		Endpoint:  &api.Endpoint{Host: &host, Port: &port},
	}
	target := &common.ConnectionTarget{ConnectionService: svc, CredentialService: svc}

	cmd := &cobra.Command{}
	cmd.SetErr(io.Discard)
	cmd.SetOut(io.Discard)

	if _, err := buildConnectionDetailsForTarget(cmd, testConfig(t), target, common.ConnectionDetailsOptions{Pooled: true, Role: "tsdbadmin"}); err == nil {
		t.Fatal("expected an error when a pooler is unavailable for the primary, got nil")
	}
}
