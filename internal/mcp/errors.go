package mcp

// readOnlyGatedTools are the service-mutating tools addTool skips in read-only mode.
var readOnlyGatedTools = []string{
	toolServiceCreate,
	toolServiceFork,
	toolServiceStart,
	toolServiceStop,
	toolServiceResize,
	toolServiceUpdatePassword,
}
