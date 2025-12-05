package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"go.uber.org/zap"
)

// Capabilities holds all MCP server capabilities
type Capabilities struct {
	Tools             []*mcp.Tool             `json:"tools" yaml:"tools"`
	Prompts           []*mcp.Prompt           `json:"prompts" yaml:"prompts"`
	Resources         []*mcp.Resource         `json:"resources" yaml:"resources"`
	ResourceTemplates []*mcp.ResourceTemplate `json:"resource_templates" yaml:"resource_templates"`
}

// ListCapabilities creates a temporary in-memory client connection to list all
// capabilities (tools, prompts, resources, and resource templates).
func (s *Server) ListCapabilities(ctx context.Context) (*Capabilities, error) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    ServerName,
		Version: config.Version,
	}, nil)

	serverSession, err := s.mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect server: %w", err)
	}
	defer serverSession.Close()

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect client: %w", err)
	}
	defer clientSession.Close()

	capabilities := &Capabilities{
		Tools:             []*mcp.Tool{},
		Prompts:           []*mcp.Prompt{},
		Resources:         []*mcp.Resource{},
		ResourceTemplates: []*mcp.ResourceTemplate{},
	}

	for tool, err := range clientSession.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("failed to list tools: %w", err)
		}
		capabilities.Tools = append(capabilities.Tools, tool)
	}

	for prompt, err := range clientSession.Prompts(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("failed to list prompts: %w", err)
		}
		capabilities.Prompts = append(capabilities.Prompts, prompt)
	}

	for resource, err := range clientSession.Resources(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("failed to list resources: %w", err)
		}
		capabilities.Resources = append(capabilities.Resources, resource)
	}

	for template, err := range clientSession.ResourceTemplates(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("failed to list resource templates: %w", err)
		}
		capabilities.ResourceTemplates = append(capabilities.ResourceTemplates, template)
	}

	if err := clientSession.Close(); err != nil {
		logging.Error("Error closing client session", zap.Error(err))
	}

	if err := serverSession.Close(); err != nil {
		logging.Error("Error closing server session", zap.Error(err))
	}

	return capabilities, nil
}
