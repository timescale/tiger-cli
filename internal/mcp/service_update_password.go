package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceUpdatePasswordInput represents input for service_update_password
type ServiceUpdatePasswordInput struct {
	ServiceID string `json:"service_id"`
	Password  string `json:"password"`
}

func (ServiceUpdatePasswordInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceUpdatePasswordInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["password"].Description = "The new password for the 'tsdbadmin' user. Must be strong and secure."
	schema.Properties["password"].Examples = []any{"MySecurePassword123!"}

	return schema
}

// ServiceUpdatePasswordOutput represents output for service_update_password
type ServiceUpdatePasswordOutput struct {
	Message         string                        `json:"message"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceUpdatePasswordOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceUpdatePasswordOutput](nil))
}

func newServiceUpdatePasswordTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceUpdatePassword,
		Title: "Update Service Password",
		Description: "Update master password for 'tsdbadmin' user of a database service. " +
			"Takes effect immediately. May terminate existing connections.",
		InputSchema:  ServiceUpdatePasswordInput{}.Schema(),
		OutputSchema: ServiceUpdatePasswordOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: util.Ptr(true), // Modifies authentication credentials
			IdempotentHint:  true,           // Same password can be set multiple times
			OpenWorldHint:   util.Ptr(true),
			Title:           "Update Service Password",
		},
	}
}

// handleServiceUpdatePassword handles the service_update_password MCP tool
func (s *Server) handleServiceUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input ServiceUpdatePasswordInput) (*mcp.CallToolResult, ServiceUpdatePasswordOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	if err := common.CheckReadOnly(cfg); err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	s.logger.Info("MCP: Updating service password",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID))

	// Prepare password update request
	updateReq := api.UpdatePasswordInput{
		Password: input.Password,
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Fetch first so we can reject read replicas and reuse the service for
	// password storage below.
	serviceResp, err := client.GetServiceWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}
	if serviceResp.StatusCode() != http.StatusOK {
		return nil, ServiceUpdatePasswordOutput{}, common.ExitWithErrorFromStatusCode(serviceResp.StatusCode(), serviceResp.JSON4XX)
	}
	if serviceResp.JSON200 == nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("empty response from API")
	}
	service := *serviceResp.JSON200
	if common.IsReadReplica(service) {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("%q is a read replica; update the password on its primary service %q instead",
			input.ServiceID, util.DerefStr(service.ForkedFrom.ServiceId))
	}

	resp, err := client.UpdatePasswordWithResponse(ctx, projectID, input.ServiceID, updateReq)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("failed to update service password: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return nil, ServiceUpdatePasswordOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	// Save the new password using the service we already fetched.
	result, saveErr := common.SavePasswordWithResult(cfg, service, input.Password, "tsdbadmin")
	passwordStorage := &result
	if saveErr != nil {
		s.logger.Warn("MCP: Password storage failed", slog.Any("error", saveErr))
	} else {
		s.logger.Info("MCP: Password saved successfully", slog.String("method", result.Method))
	}

	output := ServiceUpdatePasswordOutput{
		Message:         "Master password for 'tsdbadmin' user updated successfully",
		PasswordStorage: passwordStorage,
	}

	return nil, output, nil
}
