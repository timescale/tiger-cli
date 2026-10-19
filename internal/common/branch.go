package common

import (
	"context"

	"github.com/timescale/tiger-cli/internal/api"
)

// BranchEnvironmentTag is the tag every branch is judged by. Branches have no
// environment of their own and always count as DEV, so read_only=prod leaves
// them writable even when their parent is tagged PROD: branching production
// to work against it safely is the main use case. read_only=all still blocks
// them.
const BranchEnvironmentTag = api.EnvironmentTagDEV

// BranchCPUMemoryConfigs returns the CPU/Memory configurations a branch can be
// created or resized onto. Only fluid services can be branched, so these are
// the fluid platform's.
func BranchCPUMemoryConfigs() CPUMemoryConfigs {
	return FluidCPUMemoryConfigs()
}

// BranchAsService adapts a branch to the api.Service shape that password
// storage and connection details work with. Both key off the ID, project, and
// endpoint, which a branch has just like a service.
func BranchAsService(branch api.Branch) api.Service {
	return api.Service{
		ServiceID: branch.ID,
		ProjectID: branch.ProjectID,
		Name:      branch.Name,
		Status:    branch.Status,
		Created:   branch.Created,
		Endpoint:  branch.Endpoint,
	}
}

// ResolveBranch resolves ref, a branch ID or name, to the branch it names.
// The branch carries its parent's ID, which every other branch endpoint needs.
func ResolveBranch(ctx context.Context, client api.ClientWithResponsesInterface, projectID, ref string) (*api.Branch, error) {
	resolved, err := Resolve(ctx, client, projectID, ref, new(api.ResolveTypeBRANCH))
	if err != nil {
		return nil, err
	}
	return resolved.Branch, nil
}
