package common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// ConnectionTarget describes where to connect and whose credentials to use.
// Connect supplies the endpoint/pooler, Credential the password — the same
// service for a primary; for a read replica, Connect is the replica and
// Credential is the parent primary whose credentials it shares.
type ConnectionTarget struct {
	Connect    api.Service
	Credential api.Service
	IsReplica  bool
}

// Details builds connection details for the target — endpoint/pooler from
// Connect, password from Credential. A requested-but-unavailable pooler is a
// hard error for a primary but silently falls back to direct for a replica.
func (t *ConnectionTarget) Details(opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	details, err := GetConnectionDetailsFor(t.Connect, t.Credential, opts)
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

// ResolveConnectionTarget turns a fetched service into a ConnectionTarget. When
// the service is a standby read replica (ForkedFrom.IsStandby), it connects to
// the replica but resolves credentials against the parent primary, which is
// fetched here.
func ResolveConnectionTarget(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, service api.Service) (*ConnectionTarget, error) {
	fork := service.ForkedFrom
	if fork == nil || !util.Deref(fork.IsStandby) {
		return &ConnectionTarget{Connect: service, Credential: service}, nil
	}

	parentID := util.DerefStr(fork.ServiceId)
	if parentID == "" {
		return &ConnectionTarget{Connect: service, Credential: service, IsReplica: true}, nil
	}

	parent, err := GetService(ctx, client, projectID, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent service %q for read replica: %w", parentID, err)
	}
	return &ConnectionTarget{Connect: service, Credential: *parent, IsReplica: true}, nil
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
	return &ConnectionTarget{
		Connect: api.Service{
			ServiceId:        replica.Id,
			Name:             replica.Name,
			Endpoint:         replica.Endpoint,
			ConnectionPooler: replica.ConnectionPooler,
		},
		Credential: primary,
		IsReplica:  true,
	}
}
