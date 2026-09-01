package mcp

// readOnlyGatedTools are the service-mutating tools addTool skips under
// read_only=all.
var readOnlyGatedTools = []string{
	toolServiceCreate,
	toolServiceFork,
	toolServiceStart,
	toolServiceStop,
	toolServiceResize,
	toolServiceUpdatePassword,
}
