package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestAuthStatusCmd(t *testing.T) {
	patInfo := api.AuthInfo{
		Type: api.AuthInfoTypeAPIKey,
		APIKey: &api.AuthInfoAPIKey{
			Name:      "Test Credentials",
			PublicKey: "test-public-key",
			Created:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			Project: api.AuthInfoProject{
				ID:       "test-project-123",
				Name:     "Test Project",
				PlanType: "free",
			},
			IssuingUser: api.AuthInfoIssuingUser{
				ID:    "user-123",
				Name:  "Test User",
				Email: "test@example.com",
			},
		},
	}
	oauthInfo := api.AuthInfo{
		Type: api.AuthInfoTypeOauth,
		Oauth: &api.AuthInfoOAuth{
			User: api.AuthInfoUser{
				ID:    "user-123",
				Name:  "Test User",
				Email: "test@example.com",
			},
		},
	}
	setupAuthInfo := func(info api.AuthInfo) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetAuthInfoWithResponse(validCtx).
				Return(&api.GetAuthInfoResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &info,
				}, nil)
		}
	}
	checkExitCode := func(want int) func(t *testing.T, result cmdResult) {
		return func(t *testing.T, result cmdResult) {
			t.Helper()
			var exitErr common.ExitCodeError
			if !errors.As(result.err, &exitErr) {
				t.Fatalf("expected ExitCodeError, got: %v", result.err)
			}
			if exitErr.ExitCode() != want {
				t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), want)
			}
		}
	}

	patTable := `┌─────────────────┬─────────────────────────────────┐
│    PROPERTY     │              VALUE              │
├─────────────────┼─────────────────────────────────┤
│ Status          │ Logged in                       │
│ Credential Name │ Test Credentials                │
│ Public Key      │ test-public-key                 │
│ Created At      │ 2025-01-15 10:30:00 UTC         │
│ Project         │ Test Project (test-project-123) │
│ Plan Type       │ Free                            │
│ Issuing User    │ Test User (test@example.com)    │
└─────────────────┴─────────────────────────────────┘
`

	tests := []cmdTest{
		{
			name:    "rejects positional args",
			args:    []string{"auth", "status", "extra"},
			wantErr: `unknown command "extra" for "tiger auth status"`,
		},
		{
			name:    "not logged in",
			args:    []string{"auth", "status"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: "not logged in",
			check:   checkExitCode(common.ExitAuthenticationError),
		},
		{
			name: "network error",
			args: []string{"auth", "status"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAuthInfoWithResponse(validCtx).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get auth information: connection refused",
		},
		{
			name: "API error",
			args: []string{"auth", "status"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAuthInfoWithResponse(validCtx).
					Return(&api.GetAuthInfoResponse{
						HTTPResponse: httpResponse(http.StatusUnauthorized),
						JSON4XX:      &api.Error{Message: new("invalid credentials")},
					}, nil)
			},
			wantErr: "invalid credentials",
			check:   checkExitCode(common.ExitAuthenticationError),
		},
		{
			name: "nil response body",
			args: []string{"auth", "status"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAuthInfoWithResponse(validCtx).
					Return(&api.GetAuthInfoResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "table output for PAT credentials",
			args:       []string{"auth", "status"},
			setup:      setupAuthInfo(patInfo),
			wantStdout: patTable,
		},
		{
			name:  "table output for OAuth session",
			args:  []string{"auth", "status"},
			setup: setupAuthInfo(oauthInfo),
			wantStdout: `┌─────────────┬──────────────────────────────┐
│  PROPERTY   │            VALUE             │
├─────────────┼──────────────────────────────┤
│ Status      │ Logged in                    │
│ Auth Method │ OAuth                        │
│ User        │ Test User (test@example.com) │
└─────────────┴──────────────────────────────┘
`,
		},
		{
			name: "table output for OAuth session without name",
			args: []string{"auth", "status"},
			setup: setupAuthInfo(api.AuthInfo{
				Type: api.AuthInfoTypeOauth,
				Oauth: &api.AuthInfoOAuth{
					User: api.AuthInfoUser{ID: "user-123", Email: "test@example.com"},
				},
			}),
			wantStdout: `┌─────────────┬──────────────────┐
│  PROPERTY   │      VALUE       │
├─────────────┼──────────────────┤
│ Status      │ Logged in        │
│ Auth Method │ OAuth            │
│ User        │ test@example.com │
└─────────────┴──────────────────┘
`,
		},
		{
			name:    "unsupported auth info type",
			args:    []string{"auth", "status"},
			setup:   setupAuthInfo(api.AuthInfo{Type: "bogus"}),
			wantErr: `unsupported auth info type: "bogus"`,
		},
		{
			name:  "json output",
			args:  []string{"auth", "status", "-o", "json"},
			setup: setupAuthInfo(patInfo),
			wantStdout: `{
  "api_key": {
    "created": "2025-01-15T10:30:00Z",
    "issuing_user": {
      "email": "test@example.com",
      "id": "user-123",
      "name": "Test User"
    },
    "name": "Test Credentials",
    "project": {
      "id": "test-project-123",
      "name": "Test Project",
      "plan_type": "free"
    },
    "public_key": "test-public-key"
  },
  "type": "apiKey"
}
`,
		},
		{
			name:  "yaml output",
			args:  []string{"auth", "status", "-o", "yaml"},
			setup: setupAuthInfo(patInfo),
			wantStdout: `api_key:
  created: "2025-01-15T10:30:00Z"
  issuing_user:
    email: test@example.com
    id: user-123
    name: Test User
  name: Test Credentials
  project:
    id: test-project-123
    name: Test Project
    plan_type: free
  public_key: test-public-key
type: apiKey
`,
		},
		{
			name:       "whoami alias",
			args:       []string{"auth", "whoami"},
			setup:      setupAuthInfo(patInfo),
			wantStdout: patTable,
		},
	}
	runCmdTests(t, tests)
}
