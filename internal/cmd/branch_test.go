package cmd

import (
	"net/http"
	"strings"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

// sampleBranch returns a branch of sampleService with reasonable defaults.
// Use overrides to customize specific fields.
func sampleBranch(overrides ...func(*api.Branch)) api.Branch {
	branch := api.Branch{
		ID:        "br-12345",
		ProjectID: testProjectID,
		ServiceID: "svc-12345",
		Name:      "test-branch",
		Status:    api.DeployStatusREADY,
		CPUMillis: 250,
		MemoryGbs: 2,
		Creator: &api.BranchCreator{
			ID:    "42",
			Name:  "Jane Doe",
			Email: "jane.doe@example.com",
		},
		Created: time.Date(2025, 1, 16, 9, 0, 0, 0, time.UTC),
		Endpoint: &api.Endpoint{
			Host: new("br-12345.test-project-123.branch.tsdb.cloud.timescale.com"),
			Port: new(36956),
		},
	}
	for _, o := range overrides {
		o(&branch)
	}
	return branch
}

// branchStatus returns a sampleBranch override setting the branch's status.
func branchStatus(status api.DeployStatus) func(*api.Branch) {
	return func(b *api.Branch) { b.Status = status }
}

// branchType is the resolve type every `tiger branch` command restricts its
// lookup to.
var branchType = new(api.ResolveTypeBRANCH)

// expectResolveBranch registers resolving ref as a branch, answered with
// branch.
func expectResolveBranch(m *mocks.MockClientWithResponsesInterface, ref string, branch api.Branch) {
	expectResolve(m, ref, branchType, branch)
}

// expectResolveBranchStatus registers resolving ref as a branch, answered with
// status. A nil apiErr stands in for a response with no body.
func expectResolveBranchStatus(m *mocks.MockClientWithResponsesInterface, ref string, status int, apiErr *api.ClientError) {
	expectResolveStatus(m, ref, branchType, status, apiErr)
}

// expectGetBranch registers one fetch of branch, through its parent, as a
// wait loop's poll makes it.
func expectGetBranch(m *mocks.MockClientWithResponsesInterface, branch api.Branch) {
	m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, branch.ServiceID, branch.ID).
		Return(&api.GetBranchResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      &branch,
		}, nil)
}

// sampleBranchEnv is the env output of sampleBranch.
const sampleBranchEnv = `PGHOST=br-12345.test-project-123.branch.tsdb.cloud.timescale.com
PGPORT=36956
PGDATABASE=tsdb
PGUSER=tsdbadmin
`

// sampleBranchTable, sampleBranchJSON, and sampleBranchYAML are the table,
// JSON, and YAML output of sampleBranch.
const (
	sampleBranchTable = `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                                    VALUE                                                    │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Branch ID         │ br-12345                                                                                                    │
│ Name              │ test-branch                                                                                                 │
│ Parent Service    │ test-service (svc-12345)                                                                                    │
│ Status            │ READY                                                                                                       │
│ CPU               │ 0.25 cores                                                                                                  │
│ Memory            │ 2 GB                                                                                                        │
│ Created By        │ Jane Doe (jane.doe@example.com)                                                                             │
│ Created           │ 2025-01-16 09:00:00 UTC                                                                                     │
│ Direct Endpoint   │ br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956                                             │
│ Connection String │ postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
`
	sampleBranchJSON = `{
  "cpu_millis": 250,
  "created": "2025-01-16T09:00:00Z",
  "creator": {
    "email": "jane.doe@example.com",
    "id": "42",
    "name": "Jane Doe"
  },
  "endpoint": {
    "host": "br-12345.test-project-123.branch.tsdb.cloud.timescale.com",
    "port": 36956
  },
  "id": "br-12345",
  "memory_gbs": 2,
  "name": "test-branch",
  "project_id": "test-project-123",
  "service_id": "svc-12345",
  "status": "READY",
  "service_name": "test-service",
  "role": "tsdbadmin",
  "host": "br-12345.test-project-123.branch.tsdb.cloud.timescale.com",
  "port": 36956,
  "database": "tsdb",
  "connection_string": "postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require",
  "console_url": "https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview"
}
`
	sampleBranchYAML = `connection_string: postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require
console_url: https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview
cpu_millis: 250
created: "2025-01-16T09:00:00Z"
creator:
  email: jane.doe@example.com
  id: "42"
  name: Jane Doe
database: tsdb
endpoint:
  host: br-12345.test-project-123.branch.tsdb.cloud.timescale.com
  port: 36956
host: br-12345.test-project-123.branch.tsdb.cloud.timescale.com
id: br-12345
memory_gbs: 2
name: test-branch
port: 36956
project_id: test-project-123
role: tsdbadmin
service_id: svc-12345
service_name: test-service
status: READY
`
)

// sampleBranchTableNoCreator, sampleBranchJSONNoCreator, and
// sampleBranchYAMLNoCreator are the table, JSON, and YAML output of
// sampleBranch when the API returns no creator. sampleBranchTableNamelessCreator
// is its table output when the creator has no name.
var (
	sampleBranchJSONNoCreator = strings.Replace(sampleBranchJSON, `  "creator": {
    "email": "jane.doe@example.com",
    "id": "42",
    "name": "Jane Doe"
  },
`, "", 1)
	sampleBranchYAMLNoCreator = strings.Replace(sampleBranchYAML, `creator:
  email: jane.doe@example.com
  id: "42"
  name: Jane Doe
`, "", 1)
	sampleBranchTableNoCreator = strings.Replace(sampleBranchTable,
		"│ Created By        │ Jane Doe (jane.doe@example.com)                                                                             │\n", "", 1)
	sampleBranchTableNamelessCreator = strings.Replace(sampleBranchTable,
		"│ Created By        │ Jane Doe (jane.doe@example.com)  ",
		"│ Created By        │ jane.doe@example.com             ", 1)
)

// noCreator and namelessCreator are sampleBranch overrides removing the
// branch's creator and the creator's name, respectively.
func noCreator(b *api.Branch)       { b.Creator = nil }
func namelessCreator(b *api.Branch) { b.Creator.Name = "" }
