package common

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
)

// fakeResponse mirrors the generated response types, value receivers
// included, so a helper that touched the nil response returned with a request
// error would panic here as it would in production.
type fakeResponse struct {
	status    int
	body      *string
	clientErr *api.ClientError
}

func (r fakeResponse) StatusCode() int              { return r.status }
func (r fakeResponse) GetJSON4XX() *api.ClientError { return r.clientErr }
func (r fakeResponse) GetJSON200() *string          { return r.body }
func (r fakeResponse) GetJSON201() *string          { return r.body }
func (r fakeResponse) GetJSON202() *string          { return r.body }

// TestResponseHelpers runs the same cases against every helper, each with
// the status it expects.
func TestResponseHelpers(t *testing.T) {
	helpers := []struct {
		name    string
		status  int
		hasBody bool
		call    func(*fakeResponse, error) (string, error)
	}{
		{"JSON200", http.StatusOK, true, func(r *fakeResponse, err error) (string, error) { return JSON200(r, err) }},
		{"JSON201", http.StatusCreated, true, func(r *fakeResponse, err error) (string, error) { return JSON201(r, err) }},
		{"JSON202", http.StatusAccepted, true, func(r *fakeResponse, err error) (string, error) { return JSON202(r, err) }},
		{"Status202", http.StatusAccepted, false, func(r *fakeResponse, err error) (string, error) { return "", Status202(r, err) }},
		{"Status204", http.StatusNoContent, false, func(r *fakeResponse, err error) (string, error) { return "", Status204(r, err) }},
	}

	for _, h := range helpers {
		t.Run(h.name, func(t *testing.T) {
			tests := []struct {
				name     string
				resp     *fakeResponse
				err      error
				bodyOnly bool
				want     string
				wantErr  string
				wantCode int
			}{
				{
					// The generated client returns a nil response with the error.
					name:    "request error",
					err:     context.Canceled,
					wantErr: "failed to call API: context canceled",
				},
				{
					name: "wrong status with an API error",
					resp: &fakeResponse{
						status:    http.StatusNotFound,
						clientErr: &api.ClientError{Message: new("service not found")},
					},
					wantErr:  "service not found",
					wantCode: ExitServiceNotFound,
				},
				{
					name:     "wrong status without an API error",
					resp:     &fakeResponse{status: http.StatusInternalServerError},
					wantErr:  "unknown error",
					wantCode: ExitGeneralError,
				},
				{
					name:     "missing body",
					resp:     &fakeResponse{status: h.status},
					bodyOnly: true,
					wantErr:  "empty response from API",
				},
				{
					name: "expected status",
					resp: &fakeResponse{status: h.status, body: new("body")},
					want: map[bool]string{true: "body"}[h.hasBody],
				},
			}

			for _, tt := range tests {
				if tt.bodyOnly && !h.hasBody {
					continue
				}
				t.Run(tt.name, func(t *testing.T) {
					got, err := h.call(tt.resp, tt.err)
					if tt.wantErr == "" {
						if err != nil {
							t.Fatalf("unexpected error: %v", err)
						}
						if got != tt.want {
							t.Errorf("body = %q, want %q", got, tt.want)
						}
						return
					}
					if err == nil || err.Error() != tt.wantErr {
						t.Fatalf("error = %v, want %q", err, tt.wantErr)
					}
					if tt.err != nil && !errors.Is(err, tt.err) {
						t.Errorf("errors.Is(err, %v) = false", tt.err)
					}
					if tt.wantCode != 0 {
						exitErr, ok := errors.AsType[ExitCodeError](err)
						if !ok || exitErr.ExitCode() != tt.wantCode {
							t.Errorf("exit code = %v (ok=%v), want %d", exitErr.ExitCode(), ok, tt.wantCode)
						}
					}
				})
			}
		})
	}
}
