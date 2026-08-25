package cmd

import "maps"

// noDocsProxy returns run options that disable the remote docs MCP proxy
// (plus any extra config values), so tests that build an MCP server never
// reach the network. Only natively registered tools appear in listings; the
// proxied docs tools, prompts, and resources are absent.
func noDocsProxy(extra map[string]any) []runOption {
	values := map[string]any{"docs_mcp": false}
	maps.Copy(values, extra)
	return []runOption{withConfig(values)}
}
