package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// ResolvedTarget is a connection ID resolved to either a primary service or one
// of its read replica sets. Service is always the parent (credentials and
// password resets operate on it, since replicas share the primary's
// credentials); Replica is non-nil when the ID named a read replica, whose
// endpoint connections should target.
type ResolvedTarget struct {
	Service api.Service
	Replica *api.ReadReplicaSet
}

// ErrReplicaNotFound is returned by FindReplicaByID when no replica matches the
// ID — distinct from a match that can't be used or an API failure — so callers
// can fall back instead of surfacing it.
var ErrReplicaNotFound = errors.New("no matching read replica")

// ResolveServiceOrReplica resolves id to a primary service or a read replica of
// some service in the project. It tries a direct service lookup first; on
// failure it scans services for a replica with that ID (read replicas aren't
// addressable on their own). If neither matches, the original service-lookup
// error is returned.
func ResolveServiceOrReplica(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*ResolvedTarget, error) {
	resp, err := client.GetServiceWithResponse(ctx, projectID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service details: %w", err)
	}
	if resp.StatusCode() == http.StatusOK {
		if resp.JSON200 == nil {
			return nil, fmt.Errorf("empty response from API")
		}
		return &ResolvedTarget{Service: *resp.JSON200}, nil
	}

	// Only a not-found might instead be a read replica; other failures aren't
	// worth scanning every service for. A matched-but-unusable replica surfaces
	// its own error; a true no-match falls through to the service error below.
	if resp.StatusCode() == http.StatusNotFound {
		if target, scanErr := FindReplicaByID(ctx, client, projectID, id); !errors.Is(scanErr, ErrReplicaNotFound) {
			return target, scanErr
		}
	}

	return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
}

// FindReplicaByID scans the project's services for a read replica whose ID
// matches id and returns it with its parent service. A matched replica must be
// active and expose an endpoint; a match that isn't returns a descriptive
// error. It returns ErrReplicaNotFound when no replica matches. Use it to fall
// back to replica resolution once a direct service lookup has failed.
func FindReplicaByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*ResolvedTarget, error) {
	resp, err := client.GetServicesWithResponse(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 != nil {
		services := *resp.JSON200
		for i := range services {
			if services[i].ReadReplicaSets == nil {
				continue
			}
			replicas := *services[i].ReadReplicaSets
			for j := range replicas {
				replica := &replicas[j]
				if util.DerefStr(replica.Id) != id {
					continue
				}
				if replica.Status == nil || *replica.Status != api.ReadReplicaSetStatusActive {
					status := "unknown"
					if replica.Status != nil {
						status = string(*replica.Status)
					}
					return nil, fmt.Errorf("read replica %q is not active (status: %s)", id, status)
				}
				if replica.Endpoint == nil {
					return nil, fmt.Errorf("read replica %q has no endpoint available", id)
				}
				return &ResolvedTarget{Service: services[i], Replica: replica}, nil
			}
		}
	}

	return nil, fmt.Errorf("no service or read replica found with ID %q: %w", id, ErrReplicaNotFound)
}
