package cmd

import (
	"testing"
)

func TestMCPGetCmd(t *testing.T) {
	// service_get exercises the full text layout: annotation tags, description,
	// a Parameters section, and a nested Output schema.
	serviceGetText := `Get Service Details [read-only] [open-world]

Tool name: service_get

Description:
Get detailed information for a specific database service. Returns connection endpoints, replica configuration, resource allocation, creation time, and status.

Parameters:
  • service_id (required): string - Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs.
  • with_password: boolean - Whether to include the password in the response and connection string. NEVER set to true unless the user explicitly asks for the password. (default: false)

Output:
  • service (required): object
    • connection_string (required): string - PostgreSQL connection string (password embedded only if with_password=true)
    • created: string
    • direct_endpoint: string - Direct database connection endpoint
    • id (required): string - Service identifier (10-character alphanumeric string)
    • name (required): string
    • password: string - Password for tsdbadmin user (only included if with_password=true)
    • pooler_endpoint: string - Connection pooler endpoint
    • region (required): string
    • replicas (required): integer - Number of HA replicas (0=single node/no HA, 1+=HA enabled)
    • resources: object, null
      • cpu: string - CPU allocation (e.g., '0.5 cores', '1 core')
      • memory: string - Memory allocation (e.g., '2 GB', '4 GB')
    • status (required): string - Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)
    • type (required): string

`

	// service_list takes no parameters, so its text output has no Parameters
	// section.
	serviceListText := `List Database Services [read-only] [open-world]

Tool name: service_list

Description:
List all database services in your Tiger Cloud project. Returns services with status, type, region, and resource allocation.

Output:
  • services (required): []object, null
    • created: string
    • id (required): string - Service identifier (10-character alphanumeric string)
    • name (required): string
    • region (required): string
    • resources: object, null
      • cpu: string - CPU allocation (e.g., '0.5 cores', '1 core')
      • memory: string - Memory allocation (e.g., '2 GB', '4 GB')
    • status (required): string - Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)
    • type (required): string

`

	serviceListJSON := `{
  "annotations": {
    "idempotentHint": false,
    "openWorldHint": true,
    "readOnlyHint": true,
    "title": "List Database Services"
  },
  "description": "List all database services in your Tiger Cloud project. Returns services with status, type, region, and resource allocation.",
  "inputSchema": {
    "additionalProperties": false,
    "type": "object"
  },
  "name": "service_list",
  "outputSchema": {
    "additionalProperties": false,
    "properties": {
      "services": {
        "items": {
          "additionalProperties": false,
          "properties": {
            "created": {
              "type": "string"
            },
            "id": {
              "description": "Service identifier (10-character alphanumeric string)",
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "region": {
              "type": "string"
            },
            "resources": {
              "additionalProperties": false,
              "properties": {
                "cpu": {
                  "description": "CPU allocation (e.g., '0.5 cores', '1 core')",
                  "type": "string"
                },
                "memory": {
                  "description": "Memory allocation (e.g., '2 GB', '4 GB')",
                  "type": "string"
                }
              },
              "type": [
                "null",
                "object"
              ]
            },
            "status": {
              "description": "Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)",
              "type": "string"
            },
            "type": {
              "type": "string"
            }
          },
          "required": [
            "id",
            "name",
            "status",
            "type",
            "region"
          ],
          "type": "object"
        },
        "type": [
          "null",
          "array"
        ]
      }
    },
    "required": [
      "services"
    ],
    "type": "object"
  },
  "title": "List Database Services"
}
`

	serviceListYAML := `annotations:
  idempotentHint: false
  openWorldHint: true
  readOnlyHint: true
  title: List Database Services
description: List all database services in your Tiger Cloud project. Returns services with status, type, region, and resource allocation.
inputSchema:
  additionalProperties: false
  type: object
name: service_list
outputSchema:
  additionalProperties: false
  properties:
    services:
      items:
        additionalProperties: false
        properties:
          created:
            type: string
          id:
            description: Service identifier (10-character alphanumeric string)
            type: string
          name:
            type: string
          region:
            type: string
          resources:
            additionalProperties: false
            properties:
              cpu:
                description: CPU allocation (e.g., '0.5 cores', '1 core')
                type: string
              memory:
                description: Memory allocation (e.g., '2 GB', '4 GB')
                type: string
            type:
              - "null"
              - object
          status:
            description: Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)
            type: string
          type:
            type: string
        required:
          - id
          - name
          - status
          - type
          - region
        type: object
      type:
        - "null"
        - array
  required:
    - services
  type: object
title: List Database Services
`

	metricsAvailableText := `List Available Metric Series [read-only]

Tool name: service_metrics_available

Description:
List the names of all metric series available for a service. Call this first to discover what metrics exist before fetching data with service_metrics_series.

Parameters:
  • service_id (required): string - Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs.

Output:
  • series (required): []string, null

`

	runCmdTests(t, []cmdTest{
		{
			name:    "missing argument",
			args:    []string{"mcp", "get"},
			opts:    noDocsProxy(nil),
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "not found",
			args:    []string{"mcp", "get", "nonexistent"},
			opts:    noDocsProxy(nil),
			wantErr: `capability "nonexistent" not found`,
		},
		{
			name:       "text output with parameters",
			args:       []string{"mcp", "get", "service_get"},
			opts:       noDocsProxy(nil),
			wantStdout: serviceGetText,
		},
		{
			name:       "text output without parameters",
			args:       []string{"mcp", "get", "service_list"},
			opts:       noDocsProxy(nil),
			wantStdout: serviceListText,
		},
		{
			name:       "json output",
			args:       []string{"mcp", "get", "service_list", "-o", "json"},
			opts:       noDocsProxy(nil),
			wantStdout: serviceListJSON,
		},
		{
			name:       "yaml output",
			args:       []string{"mcp", "get", "service_list", "-o", "yaml"},
			opts:       noDocsProxy(nil),
			wantStdout: serviceListYAML,
		},
		{
			name:       "describe alias",
			args:       []string{"mcp", "describe", "service_list"},
			opts:       noDocsProxy(nil),
			wantStdout: serviceListText,
		},
		{
			name:       "show alias",
			args:       []string{"mcp", "show", "service_list"},
			opts:       noDocsProxy(nil),
			wantStdout: serviceListText,
		},
		{
			name:    "read-only mode hides write tools",
			args:    []string{"mcp", "get", "service_create"},
			opts:    noDocsProxy(map[string]any{"read_only": true}),
			wantErr: `capability "service_create" not found`,
		},
		{
			name:    "experimental tool hidden by default",
			args:    []string{"mcp", "get", "service_metrics_available"},
			opts:    noDocsProxy(nil),
			wantErr: `capability "service_metrics_available" not found`,
		},
		{
			name: "experimental tool visible with gate on",
			args: []string{"mcp", "get", "service_metrics_available"},
			opts: append(noDocsProxy(nil),
				withEnv("TIGER_EXPERIMENTAL", "true")),
			wantStdout: metricsAvailableText,
		},
	})
}
