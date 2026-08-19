package common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// ConnectionTarget is the service to connect to plus the service whose
// credentials to use. They're the same for a primary; for a read replica the
// CredentialService is the parent primary, whose credentials it shares.
type ConnectionTarget struct {
	ConnectionService api.Service
	CredentialService api.Service
	IsReplica         bool
}

// ReadOnlySession reports whether read-only mode requires this target's session to
// open in Tiger Cloud's immutable read-only mode. Only ConnectionService decides,
// so a replica is judged on its own environment tag rather than its primary's.
func (t *ConnectionTarget) ReadOnlySession(cfg *config.Config) bool {
	return ForcesReadOnlySession(cfg, t.ConnectionService)
}

// Details builds the target's connection details. A requested-but-unavailable
// pooler is a hard error for a primary but silently falls back to direct for a
// replica.
func (t *ConnectionTarget) Details(cfg *config.Config, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	details, err := GetConnectionDetailsFor(cfg, t.ConnectionService, t.CredentialService, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}
	if !t.IsReplica {
		if err := details.RequirePooler(opts.Pooled); err != nil {
			return nil, err
		}
	}
	return details, nil
}

// GetService fetches a single service by ID. The API resolves both primary
// service IDs and read replica set IDs here; a read replica comes back as a
// service whose endpoint is the replica's and whose ForkedFrom links to its
// parent.
func GetService(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*api.Service, error) {
	resp, err := client.GetServiceWithResponse(ctx, projectID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service details: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from API")
	}
	return resp.JSON200, nil
}

// IsReadReplica reports whether the service is a standby read replica (which
// shares its parent primary's credentials).
func IsReadReplica(service api.Service) bool {
	return service.ForkedFrom != nil && util.Deref(service.ForkedFrom.IsStandby)
}

// ResolveConnectionTarget turns a fetched service into a ConnectionTarget. When
// the service is a standby read replica, it connects to the replica but resolves
// credentials against the parent primary, which is fetched here.
func ResolveConnectionTarget(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, service api.Service) (*ConnectionTarget, error) {
	if !IsReadReplica(service) {
		return &ConnectionTarget{ConnectionService: service, CredentialService: service}, nil
	}

	parentID := util.DerefStr(service.ForkedFrom.ServiceID)
	if parentID == "" {
		return &ConnectionTarget{ConnectionService: service, CredentialService: service, IsReplica: true}, nil
	}

	parent, err := GetService(ctx, client, projectID, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent service %q for read replica: %w", parentID, err)
	}
	return &ConnectionTarget{ConnectionService: service, CredentialService: *parent, IsReplica: true}, nil
}

// ResolveConnectionTargetByID fetches a service (which may be a read replica) by
// ID and resolves its ConnectionTarget.
func ResolveConnectionTargetByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*ConnectionTarget, error) {
	service, err := GetService(ctx, client, projectID, id)
	if err != nil {
		return nil, err
	}
	return ResolveConnectionTarget(ctx, client, projectID, *service)
}

// NewReplicaConnectionTarget builds a ConnectionTarget for connecting to one of
// a service's read replica sets (as listed via the /replicaSets endpoint). The
// replica supplies the endpoint; the primary supplies the credentials.
func NewReplicaConnectionTarget(primary api.Service, replica api.ReadReplicaSet) *ConnectionTarget {
	// A replica set's environment tag decides its own sessions, so carry it over.
	// ReplicaSetMetadata and ServiceMetadata are separate types holding the same
	// field, so it has to be copied rather than assigned.
	var metadata *api.ServiceMetadata
	if replica.Metadata != nil {
		metadata = &api.ServiceMetadata{Environment: replica.Metadata.Environment}
	}

	return &ConnectionTarget{
		ConnectionService: api.Service{
			ServiceID:        replica.ID,
			Name:             replica.Name,
			Endpoint:         replica.Endpoint,
			ConnectionPooler: replica.ConnectionPooler,
			Metadata:         metadata,
		},
		CredentialService: primary,
		IsReplica:         true,
	}
}
