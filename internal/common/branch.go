package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/api"
)

// BranchEnvironmentTag is the tag every branch is judged by. Branches have no
// environment of their own and always count as DEV, so read_only=prod leaves
// them writable even when their parent is tagged PROD: branching production
// to work against it safely is the main use case. read_only=all still blocks
// them.
const BranchEnvironmentTag = api.EnvironmentTagDEV

// ResolveBranch resolves ref, a branch ID or name, to the branch it names.
// The branch carries its parent's ID, which every other branch endpoint needs.
func ResolveBranch(ctx context.Context, client api.ClientWithResponsesInterface, projectID, ref string) (*api.Branch, error) {
	resolved, err := resolve(ctx, client, projectID, ref, new(api.ResolveTypeBRANCH), "branch")
	if err != nil {
		return nil, err
	}
	return resolved.Branch, nil
}

// ListBranches returns serviceID's branches.
func ListBranches(ctx context.Context, client api.ClientWithResponsesInterface, projectID, serviceID string) ([]api.Branch, error) {
	resp, err := client.GetBranchesWithResponse(ctx, projectID, serviceID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}
	return *resp.JSON200, nil
}
