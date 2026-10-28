package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchCreateInput represents input for branch_create
type BranchCreateInput struct {
	ServiceID    string `json:"service_id"`
	Name         string `json:"name,omitempty"`
	CPUMemory    string `json:"cpu_memory,omitempty"`
	IOPS         *int   `json:"iops,omitempty"`
	Wait         bool   `json:"wait,omitempty"`
	WithPassword bool   `json:"with_password,omitempty"`
}

func (BranchCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchCreateInput](nil))

	setServiceIDSchemaProperties(schema)
	schema.Properties["service_id"].Description = "Unique identifier of the Fluid Service to branch (10-character alphanumeric string). Use service_list to find service IDs. Branching from a branch isn't supported."

	schema.Properties["name"].Description = "Name for the branch, unique within the parent service (auto-generated if not provided)"
	schema.Properties["name"].Examples = []any{"ci-4821", "feature/user-auth"}

	setBranchCPUMemorySchemaProperties(schema, "CPU and memory allocation combination. If not specified, the branch gets 0.25 CPU/2 GB, whatever the parent service's size.")
	setBranchIOPSSchemaProperties(schema, "If not specified, the branch gets the baseline IOPS for the user's plan and its CPU allocation.")

	schema.Properties["wait"].Description = "Whether to wait for the branch to be fully ready before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying the branch. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	setWithPasswordSchemaProperties(schema)

	return schema
}

// BranchCreateOutput represents output for branch_create
type BranchCreateOutput struct {
	Branch          BranchDetail                  `json:"branch"`
	Message         string                        `json:"message"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (BranchCreateOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchCreateOutput](nil))
	schema.Properties["branch"] = BranchDetail{}.Schema()
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	schema.Properties["password_storage"].Description = "Where the branch's new tsdbadmin password was saved locally, and whether saving it succeeded"
	return schema
}

func newBranchCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchCreate,
		Title: "Create Branch",
		Description: `Create a branch of a Fluid Service.

A branch is a lightweight, ephemeral copy of a service for development, testing, CI/CD, and agent workflows. It shares the parent's storage as of the moment it is created and gets its own compute. The tsdbadmin user gets a new password, which is saved to the configured password storage. Branches are always treated as DEV, so under read-only mode for production services a PROD service can still be branched.

By default:
- The name is auto-generated
- The branch gets 0.25 CPU and 2 GB of memory, and the baseline IOPS for that size

WARNING: Creates billable resources.`,
		InputSchema:  BranchCreateInput{}.Schema(),
		OutputSchema: BranchCreateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,      // Branching the same service multiple times creates multiple branches
			OpenWorldHint:   new(false),
			Title:           "Create Branch",
		},
	}
}

// handleBranchCreate handles the branch_create MCP tool
func (s *Server) handleBranchCreate(ctx context.Context, req *mcp.CallToolRequest, input BranchCreateInput) (*mcp.CallToolResult, BranchCreateOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchCreateOutput{}, err
	}

	// Gate on the branch's own tag, which is always DEV: under prod mode,
	// branching a PROD service is allowed.
	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchCreateOutput{}, err
	}

	var createReq api.BranchCreate
	if input.Name != "" {
		createReq.Name = &input.Name
	}
	if input.CPUMemory != "" {
		cpuMemoryCfg, err := parseBranchCPUMemory(input.CPUMemory)
		if err != nil {
			return nil, BranchCreateOutput{}, err
		}
		createReq.CPUMillis = &cpuMemoryCfg.CPUMillis
		createReq.MemoryGbs = &cpuMemoryCfg.MemoryGBs
	}
	createReq.Iops = input.IOPS

	s.logger.Info("MCP: Creating branch",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("name", input.Name),
		slog.String("cpu_memory", input.CPUMemory),
		slog.Any("iops", input.IOPS),
	)

	resp, err := client.CreateBranchWithResponse(ctx, projectID, input.ServiceID, createReq)
	if err != nil {
		return nil, BranchCreateOutput{}, fmt.Errorf("failed to create branch: %w", err)
	}
	if resp.StatusCode() != http.StatusAccepted {
		return nil, BranchCreateOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON202 == nil {
		return nil, BranchCreateOutput{}, fmt.Errorf("empty response from API")
	}
	branch := *resp.JSON202

	// Saved before any waiting, so the password is stored even if the wait
	// fails or is interrupted.
	var passwordStorage *common.PasswordStorageResult
	if branch.InitialPassword != nil {
		result, err := common.SavePasswordWithResult(cfg, common.BranchPasswordTarget(branch), *branch.InitialPassword, "tsdbadmin")
		passwordStorage = &result
		if err != nil {
			s.logger.Warn("MCP: Password storage failed", slog.Any("error", err))
		}
	}

	message := "Branch creation request accepted. The branch may still be provisioning."
	if input.Wait {
		if err := common.WaitForBranch(ctx, common.WaitForBranchArgs{
			Client:       client,
			ProjectID:    projectID,
			ServiceID:    input.ServiceID,
			BranchID:     branch.ID,
			Branch:       &branch,
			TargetStatus: api.DeployStatusREADY,
			Timeout:      waitTimeout,
			TimeoutMsg:   "branch may still be provisioning",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Branch is ready."
		}
	}
	return nil, BranchCreateOutput{
		Branch:          s.convertToBranchDetail(cfg, branch, input.WithPassword),
		Message:         message,
		PasswordStorage: passwordStorage,
	}, nil
}
