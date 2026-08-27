package cmd

import (
	"encoding/json"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMCPListCmd(t *testing.T) {
	defaultTools := []string{
		"db_execute_query",
		"db_schema",
		"service_create",
		"service_fork",
		"service_get",
		"service_list",
		"service_logs",
		"service_resize",
		"service_start",
		"service_stop",
		"service_update_password",
	}

	wantText := `┌──────┬─────────────────────────┐
│ TYPE │          NAME           │
├──────┼─────────────────────────┤
│ tool │ db_execute_query        │
│ tool │ db_schema               │
│ tool │ service_create          │
│ tool │ service_fork            │
│ tool │ service_get             │
│ tool │ service_list            │
│ tool │ service_logs            │
│ tool │ service_resize          │
│ tool │ service_start           │
│ tool │ service_stop            │
│ tool │ service_update_password │
└──────┴─────────────────────────┘
`

	// Read-only mode skips the service-mutating tools at registration time.
	wantTextReadOnly := `┌──────┬──────────────────┐
│ TYPE │       NAME       │
├──────┼──────────────────┤
│ tool │ db_execute_query │
│ tool │ db_schema        │
│ tool │ service_get      │
│ tool │ service_list     │
│ tool │ service_logs     │
└──────┴──────────────────┘
`

	// TIGER_EXPERIMENTAL registers the preview backups and metrics tools.
	wantTextExperimental := `┌──────┬───────────────────────────┐
│ TYPE │           NAME            │
├──────┼───────────────────────────┤
│ tool │ db_execute_query          │
│ tool │ db_schema                 │
│ tool │ service_backups           │
│ tool │ service_create            │
│ tool │ service_fork              │
│ tool │ service_get               │
│ tool │ service_list              │
│ tool │ service_logs              │
│ tool │ service_metrics_available │
│ tool │ service_metrics_series    │
│ tool │ service_resize            │
│ tool │ service_start             │
│ tool │ service_stop              │
│ tool │ service_update_password   │
└──────┴───────────────────────────┘
`

	// matchCapabilities parses structured (JSON/YAML) output and verifies the
	// capability structure and the exact set of listed tools. Full schemas are
	// too large to exact-match; the text cases pin the capability list.
	matchCapabilities := func(unmarshal func([]byte, any) error) matcher {
		return matchFunc(func(t *testing.T, got string) {
			var capabilities map[string]any
			if err := unmarshal([]byte(got), &capabilities); err != nil {
				t.Fatalf("failed to parse output: %v", err)
			}

			for _, key := range []string{"tools", "prompts", "resources", "resource_templates"} {
				if _, ok := capabilities[key]; !ok {
					t.Errorf("output missing %q key", key)
				}
			}

			tools, ok := capabilities["tools"].([]any)
			if !ok {
				t.Fatalf("tools is not an array: %T", capabilities["tools"])
			}

			var names []string
			for _, item := range tools {
				tool, ok := item.(map[string]any)
				if !ok {
					t.Fatalf("tool is not an object: %T", item)
				}
				name, _ := tool["name"].(string)
				if name == "" {
					t.Error("tool missing name field")
				}
				if desc, _ := tool["description"].(string); desc == "" {
					t.Errorf("tool %q missing description field", name)
				}
				names = append(names, name)
			}
			if !slices.Equal(names, defaultTools) {
				t.Errorf("tool names mismatch:\ngot:  %v\nwant: %v", names, defaultTools)
			}
		})
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "invalid output format",
			args:    []string{"mcp", "list", "-o", "invalid"},
			opts:    noDocsProxy(nil),
			wantErr: `invalid argument "invalid" for "-o, --output" flag: invalid output format: invalid (must be one of: json, yaml, table)`,
		},
		{
			name:       "text output",
			args:       []string{"mcp", "list"},
			opts:       noDocsProxy(nil),
			wantStdout: wantText,
		},
		{
			name:       "ls alias",
			args:       []string{"mcp", "ls"},
			opts:       noDocsProxy(nil),
			wantStdout: wantText,
		},
		{
			name:       "json output",
			args:       []string{"mcp", "list", "-o", "json"},
			opts:       noDocsProxy(nil),
			wantStdout: matchCapabilities(json.Unmarshal),
		},
		{
			name:       "yaml output",
			args:       []string{"mcp", "list", "-o", "yaml"},
			opts:       noDocsProxy(nil),
			wantStdout: matchCapabilities(yaml.Unmarshal),
		},
		{
			name:       "read-only mode skips write tools",
			args:       []string{"mcp", "list"},
			opts:       noDocsProxy(map[string]any{"read_only": true}),
			wantStdout: wantTextReadOnly,
		},
		{
			name: "experimental adds metrics tools",
			args: []string{"mcp", "list"},
			opts: append(noDocsProxy(nil),
				withEnv("TIGER_EXPERIMENTAL", "true")),
			wantStdout: wantTextExperimental,
		},
	})
}
