package common

import (
	"context"
	"fmt"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/util"
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

// CanHaveBranches reports whether service can have branches, so callers can
// skip listing them for services that never do. Only fluid services can be
// branched.
func CanHaveBranches(service api.Service) bool {
	return util.Deref(service.Platform) == api.ServicePlatformFLUID
}

// FormatBranchCount phrases a number of branches, as "1 branch" or
// "2 branches".
func FormatBranchCount(n int) string {
	if n == 1 {
		return "1 branch"
	}
	return fmt.Sprintf("%d branches", n)
}
