package mcp

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchLogsInput represents input for branch_logs
type BranchLogsInput struct {
	BranchID string    `json:"branch_id"`
	Tail     int       `json:"tail,omitempty"`
	Since    time.Time `json:"since,omitzero"`
	Until    time.Time `json:"until,omitzero"`
}

func (BranchLogsInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchLogsInput](nil))
	setBranchIDSchemaProperties(schema)
	setLogsInputSchemaProperties(schema)
	return schema
}

// BranchLogsOutput represents output for branch_logs
type BranchLogsOutput struct {
	Logs []string `json:"logs"`
}

func (BranchLogsOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchLogsOutput](nil))
	setLogsOutputSchemaProperties(schema)
	return schema
}

func newBranchLogsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchLogs,
		Title: "Get Branch Logs",
		Description: `View logs for a branch of a database service.

Fetches and displays logs from the specified branch. By default, shows the last 100 log entries.

Supports filtering by time (via since/until parameters).`,
		InputSchema:  BranchLogsInput{}.Schema(),
		OutputSchema: BranchLogsOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "Get Branch Logs",
		},
	}
}

// handleBranchLogs handles the branch_logs MCP tool
func (s *Server) handleBranchLogs(ctx context.Context, req *mcp.CallToolRequest, input BranchLogsInput) (*mcp.CallToolResult, BranchLogsOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, BranchLogsOutput{}, err
	}

	s.logger.Info("MCP: Fetching branch logs",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID),
		slog.Int("tail", input.Tail),
		slog.Any("since", input.Since),
		slog.Any("until", input.Until),
	)

	// The logs endpoint is nested under the branch's parent service.
	branch, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchLogsOutput{}, err
	}

	entries, err := common.FetchBranchLogs(ctx, common.FetchBranchLogsArgs{
		Client:    client,
		ProjectID: projectID,
		ServiceID: branch.ServiceID,
		BranchID:  branch.ID,
		Tail:      input.Tail,
		Since:     input.Since,
		Until:     input.Until,
	})
	if err != nil {
		return nil, BranchLogsOutput{}, err
	}

	return nil, BranchLogsOutput{Logs: formatLogLines(entries)}, nil
}
