package api_test

import (
	"context"
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/api/mocks"
)

func TestValidateAPIKeyWithClient(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mocks.MockClientWithResponsesInterface)
		expectedError  string
	}{
		{
			name: "valid API key - 200 response",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
					}, nil)
			},
			expectedError: "",
		},
		{
			name: "valid API key - 404 response (project not found)",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 404},
					}, nil)
			},
			expectedError: "",
		},
		{
			name: "invalid API key - 401 response",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 401},
					}, nil)
			},
			expectedError: "invalid API key: authentication failed",
		},
		{
			name: "invalid API key - 403 response",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 403},
					}, nil)
			},
			expectedError: "invalid API key: authentication failed",
		},
		{
			name: "unexpected response - 500",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 500},
					}, nil)
			},
			expectedError: "unexpected API response: 500",
		},
		{
			name: "network error",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(nil, context.DeadlineExceeded)
			},
			expectedError: "API call failed: context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockClientWithResponsesInterface(ctrl)
			tt.setupMock(mockClient)

			err := api.ValidateAPIKeyWithClient(mockClient, "")

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if err.Error() != tt.expectedError {
					t.Errorf("Expected error %q, got %q", tt.expectedError, err.Error())
				}
			}
		})
	}
}

// TestValidateAPIKey_Integration would be an integration test that actually calls the API
// This should be run with a real API key for integration testing
func TestValidateAPIKey_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// This test would require a real API key and network connectivity
	t.Skip("Integration test requires real API key - implement when needed")
}