package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"go.uber.org/zap"
)

// CapabilityType represents a type of MCP capability
type CapabilityType string

const (
	CapabilityTypeTool             CapabilityType = "tool"
	CapabilityTypePrompt           CapabilityType = "prompt"
	CapabilityTypeResource         CapabilityType = "resource"
	CapabilityTypeResourceTemplate CapabilityType = "resource_template"
)

// String returns the string representation of the capability type
func (t CapabilityType) String() string {
	return string(t)
}

type CapabilityTypes []CapabilityType

func (t CapabilityTypes) Strings() []string {
	strs := make([]string, len(t))
	for i, t := range t {
		strs[i] = t.String()
	}
	return strs
}

func (t CapabilityTypes) String() string {
	return strings.Join(t.Strings(), ", ")
}

func ValidCapabilityTypes() CapabilityTypes {
	return CapabilityTypes{
		CapabilityTypeTool,
		CapabilityTypePrompt,
		CapabilityTypeResource,
		CapabilityTypeResourceTemplate,
	}
}

// ValidateCapabilityType validates that a string is a valid capability type
// Returns the CapabilityType and nil if valid, or an error if invalid
func ValidateCapabilityType(s string) (CapabilityType, error) {
	types := ValidCapabilityTypes()
	for _, valid := range types {
		if s == string(valid) {
			return valid, nil
		}
	}
	return "", fmt.Errorf("invalid capability type %q, must be one of: %s", s, types)
}

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

// GetTool finds a tool by name, returns the tool and true if found, nil and false otherwise
func (c *Capabilities) GetTool(name string) (*mcp.Tool, bool) {
	for _, tool := range c.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return nil, false
}

// GetPrompt finds a prompt by name, returns the prompt and true if found, nil and false otherwise
func (c *Capabilities) GetPrompt(name string) (*mcp.Prompt, bool) {
	for _, prompt := range c.Prompts {
		if prompt.Name == name {
			return prompt, true
		}
	}
	return nil, false
}

// GetResource finds a resource by name, returns the resource and true if found, nil and false otherwise
func (c *Capabilities) GetResource(name string) (*mcp.Resource, bool) {
	for _, resource := range c.Resources {
		if resource.Name == name {
			return resource, true
		}
	}
	return nil, false
}

// GetResourceTemplate finds a resource template by name, returns the template and true if found, nil and false otherwise
func (c *Capabilities) GetResourceTemplate(name string) (*mcp.ResourceTemplate, bool) {
	for _, template := range c.ResourceTemplates {
		if template.Name == name {
			return template, true
		}
	}
	return nil, false
}
