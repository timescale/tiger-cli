package common

import (
	"context"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
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

// GetBranch fetches a single branch by ID.
func GetBranch(ctx context.Context, client api.ClientWithResponsesInterface, projectID, branchID string) (*api.Branch, error) {
	resp, err := client.GetBranchWithResponse(ctx, projectID, branchID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch branch details: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from API")
	}
	return resp.JSON200, nil
}

// CopyParentPassword saves the parent service's stored password under the
// branch. A branch starts with its parent's roles and passwords, but the
// platform doesn't store passwords, so it can't return one on create; copying
// it up front gives the branch its own entry, which can diverge from the
// parent's once the branch's password is changed.
func CopyParentPassword(cfg *config.Config, parent api.Service, branch api.Branch, role string) (PasswordStorageResult, error) {
	storage := GetPasswordStorage(cfg)
	if _, ok := storage.(*NoStorage); ok {
		return PasswordStorageResult{
			Success: false,
			Method:  "none",
			Message: "Password not saved (--password-storage=none)",
		}, nil
	}

	password, err := GetPassword(cfg, parent, role)
	if err != nil {
		return PasswordStorageResult{
			Success: false,
			Method:  cfg.PasswordStorage,
			Message: fmt.Sprintf("Failed to copy the password of parent service %s: %s", parent.ServiceID, err),
		}, err
	}

	return SavePasswordWithResult(cfg, BranchAsService(branch), password, role)
}
