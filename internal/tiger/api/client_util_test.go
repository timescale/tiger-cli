package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/util"
	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/api/mocks"
)

func TestValidateAPIKeyWithClient(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*mocks.MockClientWithResponsesInterface)
		expectedError string
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

func TestFormatAPIError(t *testing.T) {
	tests := []struct {
		name     string
		apiErr   *api.Error
		fallback string
		expected string
	}{
		{
			name: "API error with message",
			apiErr: &api.Error{
				Code:    util.Ptr("ENTITLEMENT_ERROR"),
				Message: util.Ptr("Unauthorized. Entitlement check has failed."),
			},
			fallback: "fallback message",
			expected: "Unauthorized. Entitlement check has failed.",
		},
		{
			name:     "nil API error",
			apiErr:   nil,
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name: "API error with nil message",
			apiErr: &api.Error{
				Code:    util.Ptr("ERROR_CODE"),
				Message: nil,
			},
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name: "API error with empty message",
			apiErr: &api.Error{
				Code:    util.Ptr("ERROR_CODE"),
				Message: util.Ptr(""),
			},
			fallback: "fallback message",
			expected: "fallback message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := api.FormatAPIError(tt.apiErr, tt.fallback)
			if err.Error() != tt.expected {
				t.Errorf("Expected error message '%s', got '%s'", tt.expected, err.Error())
			}
		})
	}
}

func TestFormatAPIErrorFromBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		fallback string
		expected string
	}{
		{
			name:     "valid JSON with error message",
			body:     []byte(`{"code":"ENTITLEMENT_ERROR","message":"Unauthorized. Entitlement check has failed."}`),
			fallback: "fallback message",
			expected: "Unauthorized. Entitlement check has failed.",
		},
		{
			name:     "empty body",
			body:     []byte{},
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "nil body",
			body:     nil,
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "invalid JSON",
			body:     []byte(`{invalid json}`),
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "valid JSON with empty message",
			body:     []byte(`{"code":"ERROR_CODE","message":""}`),
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "valid JSON with null message",
			body:     []byte(`{"code":"ERROR_CODE","message":null}`),
			fallback: "fallback message",
			expected: "fallback message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := api.FormatAPIErrorFromBody(tt.body, tt.fallback)
			if err.Error() != tt.expected {
				t.Errorf("Expected error message '%s', got '%s'", tt.expected, err.Error())
			}
		})
	}
}
