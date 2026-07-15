package common

import (
	"context"
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

	// Not a service — maybe a read replica ID.
	if target, scanErr := FindReplicaByID(ctx, client, projectID, id); scanErr == nil {
		return target, nil
	}

	// Neither; surface the original service-lookup error.
	return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
}

// FindReplicaByID scans the project's services and returns the parent service
// and read replica set whose ID matches id, or an error when no replica in the
// project matches. Use it to fall back to replica resolution once a direct
// service lookup has failed.
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
				if util.DerefStr(replicas[j].Id) == id {
					return &ResolvedTarget{Service: services[i], Replica: &replicas[j]}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no service or read replica found with ID %q", id)
}
