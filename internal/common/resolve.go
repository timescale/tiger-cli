package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/api"
)

// Resolved is what a ref resolved to. Exactly one of its fields is set.
type Resolved struct {
	Service        *api.Service
	Branch         *api.Branch
	ReadReplicaSet *api.ReadReplicaSet
}

// ID returns the ID of whatever the ref resolved to.
func (r *Resolved) ID() string {
	switch {
	case r.Service != nil:
		return r.Service.ServiceID
	case r.Branch != nil:
		return r.Branch.ID
	case r.ReadReplicaSet != nil:
		return r.ReadReplicaSet.ID
	}
	return ""
}

// resolve resolves ref, an ID or a name, to the service, branch, or read
// replica set it names, considering only resources of the given type when it
// is non-nil. kind names what the caller is looking for, in errors. A ref that
// matches nothing is an ExitServiceNotFound error, and one that matches more
// than one resource is refused by the API with a 400.
func resolve(ctx context.Context, client api.ClientWithResponsesInterface, projectID, ref string, resolveType *api.ResolveType, kind string) (*Resolved, error) {
	resp, err := client.ResolveRefWithResponse(ctx, projectID, &api.ResolveRefParams{Ref: ref, Type: resolveType})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s '%s': %w", kind, ref, err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, ExitWithCode(ExitServiceNotFound, fmt.Errorf("%s '%s' not found", kind, ref))
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	resolved, err := unpackResolveResult(*resp.JSON200)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s '%s': %w", kind, ref, err)
	}

	// The API only returns the requested type, so a mismatch means the filter
	// was ignored, and acting on the result would act on the wrong thing.
	if resolveType != nil && resolvedType(resolved) != *resolveType {
		return nil, fmt.Errorf("expected a %s for '%s', got a %s", kind, ref, resolveKind(resolvedType(resolved)))
	}
	return resolved, nil
}

// resolveKind names a resolve type, in messages.
func resolveKind(resolveType api.ResolveType) string {
	switch resolveType {
	case api.ResolveTypeBRANCH:
		return "branch"
	case api.ResolveTypeREADREPLICASET:
		return "read replica set"
	default:
		return "service"
	}
}

func resolvedType(r *Resolved) api.ResolveType {
	switch {
	case r.Branch != nil:
		return api.ResolveTypeBRANCH
	case r.ReadReplicaSet != nil:
		return api.ResolveTypeREADREPLICASET
	default:
		return api.ResolveTypeSERVICE
	}
}

// unpackResolveResult decodes the union by its discriminator. Each Resolved*
// type is its plain counterpart plus the discriminator, so the raw JSON is
// decoded straight into the plain type rather than copied field by field.
func unpackResolveResult(result api.ResolveResult) (*Resolved, error) {
	discriminator, err := result.Discriminator()
	if err != nil {
		return nil, err
	}
	raw, err := result.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var resolved Resolved
	switch api.ResolveType(discriminator) {
	case api.ResolveTypeSERVICE:
		resolved.Service = &api.Service{}
		err = json.Unmarshal(raw, resolved.Service)
	case api.ResolveTypeBRANCH:
		resolved.Branch = &api.Branch{}
		err = json.Unmarshal(raw, resolved.Branch)
	case api.ResolveTypeREADREPLICASET:
		resolved.ReadReplicaSet = &api.ReadReplicaSet{}
		err = json.Unmarshal(raw, resolved.ReadReplicaSet)
	default:
		return nil, fmt.Errorf("unknown resolved type '%s'", discriminator)
	}
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}
