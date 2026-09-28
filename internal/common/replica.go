package common

import (
	"context"
	"fmt"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// ConnectionTarget is the service to connect to plus the service whose
// credentials to use. They're the same for a primary; for a read replica the
// CredentialService is the parent primary, whose credentials it shares.
type ConnectionTarget struct {
	// ConnectionService alone decides a session's read-only verdict, so a replica
	// is judged on its own environment tag rather than its primary's.
	ConnectionService api.Service
	CredentialService api.Service
	IsReplica         bool
}

// Details builds the target's connection details. A requested-but-unavailable
// pooler is a hard error for a primary but silently falls back to direct for a
// replica (see ReplicaPoolerWarning).
func (t *ConnectionTarget) Details(cfg *config.Config, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	details, err := getConnectionDetails(cfg, t.ConnectionService, t.CredentialService, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}
	if opts.Pooled && !details.IsPooler && !t.IsReplica {
		return nil, fmt.Errorf("connection pooler not available for this service")
	}
	return details, nil
}

// IsReadReplica reports whether the service is a standby read replica (which
// shares its parent primary's credentials).
func IsReadReplica(service api.Service) bool {
	return service.ForkedFrom != nil && util.Deref(service.ForkedFrom.IsStandby)
}

// ResolveConnectionTargetByID fetches the service named by id — which may be a
// primary service ID or a read replica set ID, both of which GetService
// resolves — and works out which service holds its credentials. A standby read
// replica connects to its own endpoint but shares the parent primary's
// password, so the parent is fetched here too.
func ResolveConnectionTargetByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*ConnectionTarget, error) {
	service, err := GetService(ctx, client, projectID, id)
	if err != nil {
		return nil, err
	}

	if !IsReadReplica(*service) {
		return &ConnectionTarget{ConnectionService: *service, CredentialService: *service}, nil
	}

	// A replica with no parent recorded has nowhere else to look, so it stands
	// in as its own credential service.
	parentID := util.DerefStr(service.ForkedFrom.ServiceID)
	if parentID == "" {
		return &ConnectionTarget{ConnectionService: *service, CredentialService: *service, IsReplica: true}, nil
	}

	parent, err := GetService(ctx, client, projectID, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent service %q for read replica: %w", parentID, err)
	}
	return &ConnectionTarget{ConnectionService: *service, CredentialService: *parent, IsReplica: true}, nil
}

// NewReplicaConnectionTarget builds a ConnectionTarget for connecting to one of
// a service's read replica sets (as listed via the /replicaSets endpoint). The
// replica supplies the endpoint; the primary supplies the credentials.
func NewReplicaConnectionTarget(primary api.Service, replica api.ReadReplicaSet) *ConnectionTarget {
	// Carry the replica's own tag over. ReplicaSetMetadata and ServiceMetadata are
	// separate types holding the same field, so it's copied rather than assigned.
	var metadata *api.ServiceMetadata
	if replica.Metadata != nil {
		metadata = &api.ServiceMetadata{Environment: replica.Metadata.Environment}
	}

	// Replica sets carry their own status enum. Map only "active" onto READY so
	// a readiness check on this target (CheckServiceReady) treats every other
	// state — creating, resizing, error — as not ready.
	status := api.DeployStatus("")
	if replica.Status == api.ReadReplicaSetStatusActive {
		status = api.DeployStatusREADY
	}

	return &ConnectionTarget{
		ConnectionService: api.Service{
			ServiceID:        replica.ID,
			Name:             replica.Name,
			Status:           status,
			Endpoint:         replica.Endpoint,
			ConnectionPooler: replica.ConnectionPooler,
			Metadata:         metadata,
		},
		CredentialService: primary,
		IsReplica:         true,
	}
}
