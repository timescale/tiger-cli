package common

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/api"
)

// The JSON2xx and Status2xx helpers check a Tiger Cloud API response for the
// status its endpoint is expected to return. Each takes both return values of
// a generated client call, so a call site is one call and one error check:
//
//	services, err := common.JSON200(client.GetServicesWithResponse(ctx, projectID, nil))
//
// A request error is wrapped as "failed to call API: …" (the generated
// client's *url.Error already names the method and URL). A different status
// becomes ExitWithErrorFromStatusCode with the API's own error, unprefixed,
// since its message is usually what the user should see. A missing body is
// "empty response from API". There's one helper per status, so every call
// site states the status it expects.

// statusResponse is what every generated response type provides.
type statusResponse interface {
	StatusCode() int
	GetJSON4XX() *api.ClientError
}

type json200Response[T any] interface {
	statusResponse
	GetJSON200() *T
}

type json201Response[T any] interface {
	statusResponse
	GetJSON201() *T
}

type json202Response[T any] interface {
	statusResponse
	GetJSON202() *T
}

// JSON200 returns the body of a 200 response.
func JSON200[T any](resp json200Response[T], err error) (T, error) {
	if err := checkStatus(resp, err, http.StatusOK); err != nil {
		var zero T
		return zero, err
	}
	return checkBody(resp.GetJSON200())
}

// JSON201 returns the body of a 201 response.
func JSON201[T any](resp json201Response[T], err error) (T, error) {
	if err := checkStatus(resp, err, http.StatusCreated); err != nil {
		var zero T
		return zero, err
	}
	return checkBody(resp.GetJSON201())
}

// JSON202 returns the body of a 202 response.
func JSON202[T any](resp json202Response[T], err error) (T, error) {
	if err := checkStatus(resp, err, http.StatusAccepted); err != nil {
		var zero T
		return zero, err
	}
	return checkBody(resp.GetJSON202())
}

// Status202 checks for a 202 response whose body the caller doesn't need.
func Status202(resp statusResponse, err error) error {
	return checkStatus(resp, err, http.StatusAccepted)
}

// Status204 checks for a 204 response.
func Status204(resp statusResponse, err error) error {
	return checkStatus(resp, err, http.StatusNoContent)
}

// checkStatus checks the request error before touching resp, which the
// generated client leaves nil when the request fails.
func checkStatus(resp statusResponse, err error, want int) error {
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}
	if resp.StatusCode() != want {
		return ExitWithErrorFromStatusCode(resp.StatusCode(), resp.GetJSON4XX())
	}
	return nil
}

func checkBody[T any](body *T) (T, error) {
	if body == nil {
		var zero T
		return zero, errors.New("empty response from API")
	}
	return *body, nil
}
