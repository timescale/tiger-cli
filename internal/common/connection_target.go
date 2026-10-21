package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// ConnectionTarget is what a database connection goes to: a service, one of
// its read replica sets, or a branch, reduced to what connecting to it and
// reporting on it need.
type ConnectionTarget struct {
	Kind TargetKind
	ID   string
	Name string

	// Status decides whether the target accepts connections (see CheckReady).
	Status api.DeployStatus

	// Environment alone decides a session's read-only verdict, so a replica is
	// judged on its own tag rather than its primary's, and a branch, which has
	// none, counts as DEV.
	Environment api.EnvironmentTag

	Endpoint *api.Endpoint
	Pooler   *api.ConnectionPooler

	// Credentials is whose passwords the connection uses: the target itself
	// for a service or a branch, the primary for a read replica, which shares
	// them.
	Credentials CredentialOwner
}

// CredentialOwner is the service or branch whose passwords a connection uses.
type CredentialOwner struct {
	PasswordTarget

	// Name names the owner in messages.
	Name string

	// Environment is the owner's own tag, which decides whether read-only mode
	// lets its passwords be changed.
	Environment api.EnvironmentTag
}

// TargetKind is the kind of resource a ConnectionTarget is.
type TargetKind int

const (
	TargetService TargetKind = iota
	TargetReadReplica
	TargetBranch
)

// NewServiceConnectionTarget returns the ConnectionTarget of a service,
// connecting to its own endpoint with its own credentials.
func NewServiceConnectionTarget(service api.Service) *ConnectionTarget {
	return &ConnectionTarget{
		Kind:        TargetService,
		ID:          service.ServiceID,
		Name:        service.Name,
		Status:      service.Status,
		Environment: ServiceEnvironmentTag(service),
		Endpoint:    service.Endpoint,
		Pooler:      service.ConnectionPooler,
		Credentials: ServiceCredentials(service),
	}
}

// ServiceCredentials returns a service as the owner of the credentials its
// connections use, which its read replica sets share.
func ServiceCredentials(service api.Service) CredentialOwner {
	return CredentialOwner{
		PasswordTarget: ServicePasswordTarget(service),
		Name:           service.Name,
		Environment:    ServiceEnvironmentTag(service),
	}
}

// NewBranchConnectionTarget returns the ConnectionTarget of a branch, which
// has its own credentials and no pooler.
func NewBranchConnectionTarget(branch api.Branch) *ConnectionTarget {
	return &ConnectionTarget{
		Kind:        TargetBranch,
		ID:          branch.ID,
		Name:        branch.Name,
		Status:      branch.Status,
		Environment: BranchEnvironmentTag,
		Endpoint:    branch.Endpoint,
		Credentials: CredentialOwner{
			PasswordTarget: BranchPasswordTarget(branch),
			Name:           branch.Name,
			Environment:    BranchEnvironmentTag,
		},
	}
}

// NewReplicaConnectionTarget returns the ConnectionTarget of a read replica
// set. The replica supplies the endpoint, and credentials is its primary's
// (see ServiceCredentials), since a replica shares its primary's passwords.
func NewReplicaConnectionTarget(replica api.ReadReplicaSet, credentials CredentialOwner) *ConnectionTarget {
	// Replica sets carry their own status enum. Map only "active" onto READY so
	// a readiness check on this target (CheckReady) treats every other state —
	// creating, resizing, error — as not ready.
	status := api.DeployStatus("")
	if replica.Status == api.ReadReplicaSetStatusActive {
		status = api.DeployStatusREADY
	}

	var environment *string
	if replica.Metadata != nil {
		environment = replica.Metadata.Environment
	}

	return &ConnectionTarget{
		Kind:        TargetReadReplica,
		ID:          replica.ID,
		Name:        replica.Name,
		Status:      status,
		Environment: environmentTag(environment),
		Endpoint:    replica.Endpoint,
		Pooler:      replica.ConnectionPooler,
		Credentials: credentials,
	}
}

// CheckReady returns nil only when the target accepts connections, and
// ErrPaused or ErrNotReady otherwise (see the CheckReady function).
func (t *ConnectionTarget) CheckReady() error {
	return CheckReady(t.Status)
}

// Details builds the target's connection details. A requested-but-unavailable
// pooler is a hard error for a service but silently falls back to direct for a
// replica (see ReplicaPoolerWarning).
func (t *ConnectionTarget) Details(cfg *config.Config, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	details, err := GetConnectionDetails(cfg, t, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}
	if opts.Pooled && !details.IsPooler && t.Kind != TargetReadReplica {
		return nil, fmt.Errorf("connection pooler not available for this service")
	}
	return details, nil
}

// ResolveConnectionTarget resolves ref, an ID or a name, to the service,
// branch, or read replica set it names and works out whose credentials it
// uses. Branches are a preview, so unless allowBranches is set a ref naming
// one is reported as not found.
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
		return NewBranchConnectionTarget(*resolved.Branch), nil
	case resolved.ReadReplicaSet != nil:
		replica := *resolved.ReadReplicaSet
		parent, err := GetService(ctx, client, projectID, replica.ServiceID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch parent service %q for read replica: %w", replica.ServiceID, err)
		}
		return NewReplicaConnectionTarget(replica, ServiceCredentials(*parent)), nil
	default:
		return NewServiceConnectionTarget(*resolved.Service), nil
	}
}

// ResolveConnectionTargetByID is ResolveConnectionTarget for a caller that
// takes only IDs: a ref that matched by name is reported as not found.
func ResolveConnectionTargetByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string, allowBranches bool) (*ConnectionTarget, error) {
	target, err := ResolveConnectionTarget(ctx, client, projectID, id, allowBranches)
	if err != nil {
		return nil, err
	}
	if target.ID != id {
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

// environmentTag reads an optional environment tag. Anything but PROD reads
// as DEV, since the tag is optional and treating unknowns as protected would
// have read_only=prod block untagged services.
func environmentTag(environment *string) api.EnvironmentTag {
	if strings.EqualFold(util.DerefStr(environment), string(api.EnvironmentTagPROD)) {
		return api.EnvironmentTagPROD
	}
	return api.EnvironmentTagDEV
}
