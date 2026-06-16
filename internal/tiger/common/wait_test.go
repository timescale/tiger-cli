package common

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*api.ClientWithResponses, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := api.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("failed to build client: %v", err)
	}
	return client, server.Close
}

// jsonHandler returns a handler that always responds 200 with the given JSON body.
func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}
}

func waitForRep1(t *testing.T, client *api.ClientWithResponses, timeout time.Duration) (*api.ReadReplicaSet, error) {
	t.Helper()
	return WaitForReplicaSet(context.Background(), WaitForReplicaSetArgs{
		Client:       client,
		ProjectID:    "proj-1",
		ServiceID:    "svc-1",
		ReplicaSetID: "rep-1",
		Output:       io.Discard,
		Timeout:      timeout,
	})
}

func TestWaitForReplicaSet_Status(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"becomes active", "active", false},
		{"enters error state", "error", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`[{"id":"rep-1","name":"my-replica","status":%q}]`, tc.status)
			client, closeFn := newTestClient(t, jsonHandler(body))
			defer closeFn()

			replica, err := waitForRep1(t, client, 30*time.Second)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if replica == nil || replica.Id == nil || *replica.Id != "rep-1" {
				t.Fatalf("expected active replica rep-1, got %+v", replica)
			}
		})
	}
}

func TestWaitForReplicaSet_Timeout(t *testing.T) {
	// Never becomes active.
	client, closeFn := newTestClient(t, jsonHandler(`[{"id":"rep-1","name":"my-replica","status":"creating"}]`))
	defer closeFn()

	_, err := waitForRep1(t, client, 3*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		if exitErr.ExitCode() != ExitTimeout {
			t.Errorf("expected exit code %d, got %d", ExitTimeout, exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected an ExitCode error, got %T: %v", err, err)
	}
}
