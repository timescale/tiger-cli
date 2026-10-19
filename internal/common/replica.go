package common

import (
	"context"
	"fmt"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// ConnectionTarget is the service to connect to plus the service whose
// credentials to use. They're the same for a primary or a branch; for a read
// replica the CredentialService is the parent primary, whose credentials it
// shares. A branch is carried in service form (see BranchAsService).
type ConnectionTarget struct {
	// ConnectionService alone decides a session's read-only verdict, so a replica
	// is judged on its own environment tag rather than its primary's, and a
	// branch, which has none, counts as DEV.
	ConnectionService api.Service
	CredentialService api.Service
	IsReplica         bool
	IsBranch          bool
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

// ResolveConnectionTarget resolves ref, an ID or a name, to the service,
// branch, or read replica set it names and works out which service holds its
// credentials. Branches are a preview, so unless allowBranches is set a ref
// naming one is reported as not found.
func ResolveConnectionTarget(ctx context.Context, client api.ClientWithResponsesInterface, projectID, ref string, allowBranches bool) (*ConnectionTarget, error) {
	kind := connectionTargetKind(allowBranches)
	resolved, err := resolve(ctx, client, projectID, ref, nil, kind)
	if err != nil {
		return nil, err
	}

	switch {
	case resolved.Branch != nil:
		if !allowBranches {
			return nil, ExitWithCode(ExitServiceNotFound, fmt.Errorf("%s '%s' not found", kind, ref))
		}
		branch := BranchAsService(*resolved.Branch)
		return &ConnectionTarget{ConnectionService: branch, CredentialService: branch, IsBranch: true}, nil
	case resolved.ReadReplicaSet != nil:
		replica := *resolved.ReadReplicaSet
		parent, err := GetService(ctx, client, projectID, replica.ServiceID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch parent service %q for read replica: %w", replica.ServiceID, err)
		}
		return NewReplicaConnectionTarget(*parent, replica), nil
	default:
		return NewConnectionTarget(ctx, client, projectID, *resolved.Service)
	}
}

// ResolveConnectionTargetByID is ResolveConnectionTarget for a caller that
// takes only IDs: a ref that matched by name is reported as not found.
func ResolveConnectionTargetByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string, allowBranches bool) (*ConnectionTarget, error) {
	target, err := ResolveConnectionTarget(ctx, client, projectID, id, allowBranches)
	if err != nil {
		return nil, err
	}
	if target.ConnectionService.ServiceID != id {
		return nil, ExitWithCode(ExitServiceNotFound, fmt.Errorf("%s '%s' not found", connectionTargetKind(allowBranches), id))
	}
	return target, nil
}

// connectionTargetKind names what ResolveConnectionTarget looks for, in its
// errors. Branches are left out unless they are allowed.
func connectionTargetKind(allowBranches bool) string {
	if allowBranches {
		return resolveKind(nil)
	}
	return "service or read replica set"
}

// NewConnectionTarget works out which service holds the credentials for an
// already-fetched service. A standby read replica connects to its own endpoint
// but shares the parent primary's password, so the parent is fetched here.
func NewConnectionTarget(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, service api.Service) (*ConnectionTarget, error) {
	if !IsReadReplica(service) {
		return &ConnectionTarget{ConnectionService: service, CredentialService: service}, nil
	}

	// A replica with no parent recorded has nowhere else to look, so it stands
	// in as its own credential service.
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
