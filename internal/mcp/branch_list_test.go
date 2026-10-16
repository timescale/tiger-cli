package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchListTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"service_id": "e6ue9697jf"}
	// The API lists every branch in the project, including other services'.
	other := sampleBranch(func(b *api.Branch) {
		b.ID = "0th3rbr4nc"
		b.ServiceID = "u8me885b93"
	})
	expectList := func(branches ...api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
				Return(&api.GetBranchesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &branches,
				}, nil)
		}
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchList,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_list"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchList,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a malformed service ID",
			tool:    toolBranchList,
			args:    map[string]any{"service_id": "nope"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name: "network error",
			tool: toolBranchList,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list branches: connection refused",
		},
		{
			name: "API error",
			tool: toolBranchList,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
					Return(&api.GetBranchesResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.ClientError{Message: new("forbidden")},
					}, nil)
			},
			wantErr: "forbidden",
		},
		{
			name: "nil response body",
			tool: toolBranchList,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
					Return(&api.GetBranchesResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "no branches",
			tool:       toolBranchList,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectList(other),
			wantOutput: map[string]any{"branches": []any{}},
		},
		{
			name:       "lists only the service's branches",
			tool:       toolBranchList,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectList(sampleBranch(), other),
			wantOutput: map[string]any{"branches": []any{sampleBranchDetail(api.DeployStatusREADY)}},
		},
	})
}
