package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// ConnectionTarget is what a database connection goes to: a service or one of
// its read replica sets, reduced to what connecting to it and reporting on it
// need.
type ConnectionTarget struct {
	Kind TargetKind
	ID   string
	Name string

	// Status decides whether the target accepts connections (see CheckReady).
	Status api.DeployStatus

	// Environment alone decides a session's read-only verdict, so a replica is
	// judged on its own tag rather than its primary's.
	Environment api.EnvironmentTag

	Endpoint *api.Endpoint
	Pooler   *api.ConnectionPooler

	// Credentials is whose passwords the connection uses: the target itself
	// for a service, the primary for a read replica, which shares them.
	Credentials CredentialOwner
}

// CredentialOwner is the service whose passwords a connection uses.
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

// ResolveConnectionTargetByID fetches the service named by id — which may be a
// primary service ID or a read replica set ID, both of which GetService
// resolves — and works out whose credentials it uses. Callers that already
// hold the service use NewConnectionTarget instead.
func ResolveConnectionTargetByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*ConnectionTarget, error) {
	service, err := GetService(ctx, client, projectID, id)
	if err != nil {
		return nil, err
	}
	return NewConnectionTarget(ctx, client, projectID, *service)
}

// NewConnectionTarget returns the ConnectionTarget of an already-fetched
// service. A standby read replica connects to its own endpoint but shares the
// parent primary's credentials, so the parent is fetched here.
func NewConnectionTarget(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, service api.Service) (*ConnectionTarget, error) {
	target := NewServiceConnectionTarget(service)
	if !IsReadReplica(service) {
		return target, nil
	}
	target.Kind = TargetReadReplica

	// A replica with no parent recorded has nowhere else to look, so it stands
	// in as its own credential owner.
	parentID := util.DerefStr(service.ForkedFrom.ServiceID)
	if parentID == "" {
		return target, nil
	}

	parent, err := GetService(ctx, client, projectID, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent service %q for read replica: %w", parentID, err)
	}
	target.Credentials = ServiceCredentials(*parent)
	return target, nil
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
