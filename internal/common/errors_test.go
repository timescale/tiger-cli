package common

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestCheckReadOnly(t *testing.T) {
	tests := []struct {
		mode    config.ReadOnlyMode
		tag     api.EnvironmentTag
		wantErr error
	}{
		{mode: config.ReadOnlyOff, tag: api.EnvironmentTagDEV, wantErr: nil},
		{mode: config.ReadOnlyOff, tag: api.EnvironmentTagPROD, wantErr: nil},
		{mode: config.ReadOnlyAll, tag: api.EnvironmentTagDEV, wantErr: ErrReadOnly},
		{mode: config.ReadOnlyAll, tag: api.EnvironmentTagPROD, wantErr: ErrReadOnly},
		{mode: config.ReadOnlyProd, tag: api.EnvironmentTagDEV, wantErr: nil},
		{mode: config.ReadOnlyProd, tag: api.EnvironmentTagPROD, wantErr: ErrReadOnlyProd},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.mode, tt.tag), func(t *testing.T) {
			cfg := &config.Config{ReadOnly: tt.mode}

			err := CheckReadOnly(cfg, tt.tag)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckReadOnly(read_only=%q, %q) error = %v, want %v", tt.mode, tt.tag, err, tt.wantErr)
			}
		})
	}
}

func TestCheckReadOnlyByServiceID(t *testing.T) {
	tests := []struct {
		name        string
		mode        config.ReadOnlyMode
		tag         string
		fetchStatus int
		wantErr     error
		// wantFetch records whether the check should have spent an API call.
		wantFetch bool
		// wantExitCode, when set, is the exit code the refusal must carry - see the
		// wrap in CheckReadOnlyByServiceID for why it needs re-attaching.
		wantExitCode int
	}{
		{name: "off skips the fetch", mode: config.ReadOnlyOff, tag: "PROD", wantErr: nil},
		{name: "all refuses without the fetch", mode: config.ReadOnlyAll, tag: "DEV", wantErr: ErrReadOnly},
		{name: "prod allows a DEV service", mode: config.ReadOnlyProd, tag: "DEV", wantErr: nil, wantFetch: true},
		{name: "prod refuses a PROD service", mode: config.ReadOnlyProd, tag: "PROD", wantErr: ErrReadOnlyProd, wantFetch: true},
		{name: "prod allows an untagged service", mode: config.ReadOnlyProd, tag: "", wantErr: nil, wantFetch: true},
		{
			// We can't tell whether the service is PROD, so refuse. No sentinel
			// on these, so they're asserted by exit code.
			name:         "prod refuses when the fetch fails",
			mode:         config.ReadOnlyProd,
			fetchStatus:  http.StatusInternalServerError,
			wantFetch:    true,
			wantExitCode: ExitGeneralError,
		},
		{
			name:         "prod refuses an unknown service as not found",
			mode:         config.ReadOnlyProd,
			fetchStatus:  http.StatusNotFound,
			wantFetch:    true,
			wantExitCode: ExitServiceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fetched bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fetched = true
				if tt.fetchStatus != 0 {
					w.WriteHeader(tt.fetchStatus)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.tag == "" {
					fmt.Fprint(w, `{"service_id":"svc-1"}`)
					return
				}
				fmt.Fprintf(w, `{"service_id":"svc-1","metadata":{"environment":%q}}`, tt.tag)
			}))
			defer server.Close()

			client, err := api.NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("NewClientWithResponses failed: %v", err)
			}

			cfg := &config.Config{ReadOnly: tt.mode}
			err = CheckReadOnlyByServiceID(t.Context(), cfg, client, "proj-1", "svc-1")

			if tt.wantExitCode != 0 {
				exitErr, ok := errors.AsType[ExitCodeError](err)
				if !ok {
					t.Fatalf("refusal carries no ExitCodeError: %v", err)
				}
				if got := exitErr.ExitCode(); got != tt.wantExitCode {
					t.Errorf("ExitCode() = %d, want %d (error: %v)", got, tt.wantExitCode, err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckReadOnlyByServiceID(read_only=%q, tag=%q) error = %v, want %v", tt.mode, tt.tag, err, tt.wantErr)
			}

			if fetched != tt.wantFetch {
				t.Errorf("service fetched = %t, want %t (read_only=%q)", fetched, tt.wantFetch, tt.mode)
			}
		})
	}
}

func TestExitCodeError(t *testing.T) {
	// Test the ExitCodeError type
	originalErr := fmt.Errorf("test error")
	exitErr := ExitWithCode(42, originalErr)

	if exitErr.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(ExitCodeError); ok {
		if exitCodeErr.ExitCode() != 42 {
			t.Errorf("Expected exit code 42, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("ExitWithCode should return ExitCodeError")
	}
}

func TestExitCodeError_NilError(t *testing.T) {
	exitErr := ExitWithCode(1, nil)

	if exitErr.Error() != "" {
		t.Errorf("Expected empty error message for nil error, got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("ExitWithCode should return ExitCodeError")
	}
}

func TestExitAuthenticationError(t *testing.T) {
	originalErr := fmt.Errorf("authentication failed: invalid API key")
	exitErr := ExitWithCode(ExitAuthenticationError, originalErr)

	if exitErr.Error() != "authentication failed: invalid API key" {
		t.Errorf("Expected error message 'authentication failed: invalid API key', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != ExitAuthenticationError {
			t.Errorf("Expected exit code %d (ExitAuthenticationError), got %d", ExitAuthenticationError, exitCodeErr.ExitCode())
		}
		if exitCodeErr.ExitCode() != 4 {
			t.Errorf("Expected exit code 4 for authentication error, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("ExitWithCode should return ExitCodeError with ExitCode method")
	}
}

func TestExitPermissionDenied(t *testing.T) {
	originalErr := fmt.Errorf("permission denied: insufficient access to service")
	exitErr := ExitWithCode(ExitPermissionDenied, originalErr)

	if exitErr.Error() != "permission denied: insufficient access to service" {
		t.Errorf("Expected error message 'permission denied: insufficient access to service', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != ExitPermissionDenied {
			t.Errorf("Expected exit code %d (ExitPermissionDenied), got %d", ExitPermissionDenied, exitCodeErr.ExitCode())
		}
		if exitCodeErr.ExitCode() != 5 {
			t.Errorf("Expected exit code 5 for permission denied, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("ExitWithCode should return ExitCodeError with ExitCode method")
	}
}
