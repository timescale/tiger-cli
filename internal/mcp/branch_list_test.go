package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchListTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}
	expectList := func(branches ...api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "e6ue9697jf", nil).
				Return(&api.GetBranchesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &branches,
				}, nil)
		}
	}

	runToolTests(t, []toolTest{
		{
			name:    "rejects a malformed service ID",
			tool:    toolBranchList,
			args:    map[string]any{"service_id": "nope"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchList,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name: "network error",
			tool: toolBranchList,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "e6ue9697jf", nil).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list branches: connection refused",
		},
		{
			name: "API error",
			tool: toolBranchList,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "e6ue9697jf", nil).
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
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "e6ue9697jf", nil).
					Return(&api.GetBranchesResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "lists the service's branches",
			tool:       toolBranchList,
			args:       args,
			mock:       expectList(sampleBranch()),
			wantOutput: map[string]any{"branches": []any{sampleBranchDetail(api.DeployStatusREADY)}},
		},
		{
			name:       "no branches",
			tool:       toolBranchList,
			args:       args,
			mock:       expectList(),
			wantOutput: map[string]any{"branches": []any{}},
		},
		{
			// The API leaves the creator out when the user is no longer known,
			// e.g. after being deleted.
			name:       "without a creator",
			tool:       toolBranchList,
			args:       args,
			mock:       expectList(sampleBranch(noCreator)),
			wantOutput: map[string]any{"branches": []any{sampleBranchDetailNoCreator(api.DeployStatusREADY)}},
		},
		{
			name:       "creator without a name",
			tool:       toolBranchList,
			args:       args,
			mock:       expectList(sampleBranch(namelessCreator)),
			wantOutput: map[string]any{"branches": []any{sampleBranchDetailNamelessCreator(api.DeployStatusREADY)}},
		},
	})
}
