package mcp

// readOnlyGatedTools are the service-mutating tools addTool skips under
// read_only=all. See addTool for why prod mode doesn't skip them.
var readOnlyGatedTools = []string{
	toolServiceCreate,
	toolServiceFork,
	toolServiceStart,
	toolServiceStop,
	toolServiceResize,
	toolServiceUpdatePassword,
}
