package cmd

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

// sampleReplica returns an active read replica set of sampleService, as the
// resolve endpoint returns it.
func sampleReplica(overrides ...func(*api.ReadReplicaSet)) api.ReadReplicaSet {
	replica := api.ReadReplicaSet{
		ID:        "rep-67890",
		ServiceID: "svc-12345",
		Name:      "replica-service",
		Status:    api.ReadReplicaSetStatusActive,
		Nodes:     1,
		Endpoint: &api.Endpoint{
			Host: new("rep-67890.project.tsdb.cloud.timescale.com"),
			Port: new(5432),
		},
	}
	for _, o := range overrides {
		o(&replica)
	}
	return replica
}

// expectResolveTarget registers the unrestricted resolve a `tiger db` command
// makes for what it connects to, answered with v (see expectResolve).
func expectResolveTarget(m *mocks.MockClientWithResponsesInterface, ref string, v any) {
	expectResolve(m, ref, nil, v)
}

// expectTaggedTarget is expectTaggedService for a `tiger db` command: the
// resolution of svc-12345 onto a service carrying the given environment tag.
func expectTaggedTarget(tag string) func(*mocks.MockClientWithResponsesInterface) {
	return func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveTarget(m, "svc-12345", sampleService(envTag(tag)))
	}
}

// expectResolveTargetNotFound registers a ref that matches nothing.
func expectResolveTargetNotFound(m *mocks.MockClientWithResponsesInterface, ref string) {
	expectResolveStatus(m, ref, nil, http.StatusNotFound, &api.ClientError{Message: new("resource not found")})
}

// expectResolveTargetError registers a resolution that never reaches the API.
func expectResolveTargetError(m *mocks.MockClientWithResponsesInterface, ref string, err error) {
	expectResolveError(m, ref, nil, err)
}

// expectResolveTargetStatus registers a resolution answered with status. A
// nil apiErr stands in for a response with no body.
func expectResolveTargetStatus(m *mocks.MockClientWithResponsesInterface, ref string, status int, apiErr *api.ClientError) {
	expectResolveStatus(m, ref, nil, status, apiErr)
}

// pausedMsg and notReadyMsg build the readiness errors handleDatabaseError
// returns, which name the service the command was pointed at.
func pausedMsg(serviceID string) string {
	return fmt.Sprintf("service is paused — start it with 'tiger service start %s'", serviceID)
}

func notReadyMsg(serviceID string) string {
	return fmt.Sprintf("service is not ready — check its status with 'tiger service get %s' and try again", serviceID)
}

// expectGetService expects one GetService call for id, returning svc.
func expectGetService(m *mocks.MockClientWithResponsesInterface, id string, svc api.Service) {
	m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, id).
		Return(&api.GetServiceResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      &svc,
		}, nil)
}

// TestDbCmd covers the group command itself (see groupRun and groupArgs).
func TestDbCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"db"},
			wantStdout: matchHelp("Database-specific operations including connection management, testing, and configuration."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"db", "qurey"},
			wantErr: "unknown command \"qurey\" for \"tiger db\"\n\nDid you mean this?\n\tquery",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"db", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger db"`,
		},
	})
}

// TestIsPostgresAuthenticationError covers the auth-error classifier at the helper
// level: exercising it through the command would need a real Postgres server.
func TestIsPostgresAuthenticationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"28P01 invalid_password", &pgconn.PgError{Code: "28P01"}, true},
		{"28000 invalid_authorization_specification", &pgconn.PgError{Code: "28000"}, true},
		{"57P03 cannot_connect_now", &pgconn.PgError{Code: "57P03"}, false},
		{"3D000 database does not exist", &pgconn.PgError{Code: "3D000"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPostgresAuthenticationError(tt.err); got != tt.want {
				t.Errorf("isPostgresAuthenticationError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
