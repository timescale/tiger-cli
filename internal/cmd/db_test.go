package cmd

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

// sampleReplica returns a standby read replica of sampleService, shaped the way
// GetService returns it when given a read replica set ID.
func sampleReplica(overrides ...func(*api.Service)) api.Service {
	svc := sampleService(func(s *api.Service) {
		s.ServiceID = "rep-67890"
		s.Name = "replica-service"
		s.Endpoint = &api.Endpoint{
			Host: new("rep-67890.project.tsdb.cloud.timescale.com"),
			Port: new(5432),
		}
		s.ForkedFrom = &api.ForkSpec{
			IsStandby: new(true),
			ProjectID: new(testProjectID),
			ServiceID: new("svc-12345"),
		}
	})
	for _, o := range overrides {
		o(&svc)
	}
	return svc
}

// expectGetService expects one GetService call for id, returning svc.
// pausedMsg and notReadyMsg build the readiness errors handleDatabaseError
// returns, which name the service the command was pointed at.
func pausedMsg(serviceID string) string {
	return fmt.Sprintf("service is paused — start it with 'tiger service start %s'", serviceID)
}

func notReadyMsg(serviceID string) string {
	return fmt.Sprintf("service is not ready — check its status with 'tiger service get %s' and try again", serviceID)
}

func expectGetService(m *mocks.MockClientWithResponsesInterface, id string, svc api.Service) {
	m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, id).
		Return(&api.GetServiceResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      &svc,
		}, nil)
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
