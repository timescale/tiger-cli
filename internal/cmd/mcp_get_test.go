package cmd

import (
	"testing"
)

func TestMCPGetCmd(t *testing.T) {
	// service_get exercises the full text layout: annotation tags, description,
	// a Parameters section, and a nested Output schema.
	serviceGetText := `Get Service Details [read-only]

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
    • environment (required): string - Environment tag (DEV or PROD). Under read_only=prod, services tagged PROD cannot be modified.
    • id (required): string - Service identifier (10-character alphanumeric string)
    • name (required): string
    • password: string - Password for tsdbadmin user (only included if with_password=true)
    • pooler_endpoint: string - Connection pooler endpoint
    • region (required): string
    • replicas (required): integer - Number of HA replicas (0=single node/no HA, 1+=HA enabled)
    • resources: object, null
      • cpu: string - CPU allocation
      • memory: string - Memory allocation
    • status (required): string - Service status
    • type (required): string

`

	// service_list takes no parameters, so its text output has no Parameters
	// section.
	serviceListText := `List Database Services [read-only]

Tool name: service_list

Description:
List all database services in your Tiger Cloud project. Returns services with status, type, region, and resource allocation.

Output:
  • services (required): []object, null
    • created: string
    • environment (required): string - Environment tag (DEV or PROD). Under read_only=prod, services tagged PROD cannot be modified.
    • id (required): string - Service identifier (10-character alphanumeric string)
    • name (required): string
    • region (required): string
    • resources: object, null
      • cpu: string - CPU allocation
      • memory: string - Memory allocation
    • status (required): string - Service status
    • type (required): string

`

	serviceListJSON := `{
  "annotations": {
    "idempotentHint": false,
    "openWorldHint": false,
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
            "environment": {
              "description": "Environment tag (DEV or PROD). Under read_only=prod, services tagged PROD cannot be modified.",
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
                  "description": "CPU allocation",
                  "examples": [
                    "0.5 cores",
                    "1 core",
                    "4 cores"
                  ],
                  "type": "string"
                },
                "memory": {
                  "description": "Memory allocation",
                  "examples": [
                    "2 GB",
                    "4 GB",
                    "16 GB"
                  ],
                  "type": "string"
                }
              },
              "type": [
                "null",
                "object"
              ]
            },
            "status": {
              "description": "Service status",
              "examples": [
                "READY",
                "PAUSED",
                "CONFIGURING",
                "UPGRADING"
              ],
              "type": "string"
            },
            "type": {
              "enum": [
                "TIMESCALEDB",
                "POSTGRES",
                "VECTOR"
              ],
              "type": "string"
            }
          },
          "required": [
            "id",
            "name",
            "status",
            "type",
            "region",
            "environment"
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
  openWorldHint: false
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
          environment:
            description: Environment tag (DEV or PROD). Under read_only=prod, services tagged PROD cannot be modified.
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
                description: CPU allocation
                examples:
                  - 0.5 cores
                  - 1 core
                  - 4 cores
                type: string
              memory:
                description: Memory allocation
                examples:
                  - 2 GB
                  - 4 GB
                  - 16 GB
                type: string
            type:
              - "null"
              - object
          status:
            description: Service status
            examples:
              - READY
              - PAUSED
              - CONFIGURING
              - UPGRADING
            type: string
          type:
            enum:
              - TIMESCALEDB
              - POSTGRES
              - VECTOR
            type: string
        required:
          - id
          - name
          - status
          - type
          - region
          - environment
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

	metricsDetailsText := `Get Metric Details [read-only]

Tool name: service_metrics_details

Description:
Get descriptive metadata for a metric: what it measures, its type, default
aggregation function, and available labels.

Use service_metrics_available to discover metric names, this tool to inspect
one, then service_metrics_series to fetch its data.

Parameters:
  • metric_name (required): string - Name of the metric to describe. Use service_metrics_available to discover valid names.
  • service_id (required): string - Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs.

Output:
  • details (required): object
    • default_agg (required): string, null - The aggregation function used by default when fn is omitted from a series query, or null if undocumented.
    • description (required): string - What this metric measures, or empty if undocumented.
    • labels (required): []object, null - Labels specific to this metric (e.g. datname on pg_stat_database_*) — not the region/role/ordinal labels every metric carries regardless of which one it is.
      • description (required): string - What this label identifies, or empty if undocumented.
      • name (required): string - The label's key.
    • name (required): string - Metric series name.
    • type (required): string, null - The shape of this metric's data, or null if undocumented.

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
		{
			name:    "service_metrics_details hidden by default",
			args:    []string{"mcp", "get", "service_metrics_details"},
			opts:    noDocsProxy(nil),
			wantErr: `capability "service_metrics_details" not found`,
		},
		{
			name: "service_metrics_details visible with gate on",
			args: []string{"mcp", "get", "service_metrics_details"},
			opts: append(noDocsProxy(nil),
				withEnv("TIGER_EXPERIMENTAL", "true")),
			wantStdout: metricsDetailsText,
		},
	})
}
