package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// getServiceRef determines the service ref from args or config. A ref is
// whatever the user typed to identify a service: its ID, a read replica set
// ID, or its name. The CLI never tells them apart — it passes the ref to the
// API as typed, and a ref matching more than one service comes back as an
// error rather than a guess.
func getServiceRef(cfg *config.Config, args []string) (string, error) {
	ref := cfg.ServiceID
	if len(args) > 0 {
		ref = args[0]
	}

	if ref == "" {
		return "", errors.New("service is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id-or-name>'")
	}

	return ref, nil
}

// resolveService resolves a ref to the service it names. Callers that only
// need the ID use resolveServiceID.
func resolveService(ctx context.Context, client api.ClientWithResponsesInterface, projectID, ref string) (*api.Service, error) {
	resp, err := client.ResolveServiceRefWithResponse(ctx, projectID, api.ServiceRefRequest{Ref: ref})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve service %q: %w", ref, err)
	}
	if resp.StatusCode() != http.StatusOK {
		err := common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
		// Every caller refuses an empty ref before getting here, so the only
		// 400 left is a ref matching more than one service. The server's
		// message doesn't name the candidates, so point at the command that
		// lists them.
		if resp.StatusCode() == http.StatusBadRequest {
			return nil, fmt.Errorf("%w\nRun 'tiger service list' to find the ID you want", err)
		}
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}
	return resp.JSON200, nil
}

// resolveServiceForWrite is resolveService for a command that changes the
// service it resolves. It refuses the blanket read-only case before the
// network call and gates on the tag the resolution returns, so neither half
// of the gate can be skipped or put in the wrong order at a call site.
//
// Commands that only read a service use resolveService. So does service fork:
// it resolves a source but gates on the environment it is about to request,
// not on the source's.
func resolveServiceForWrite(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface, projectID, ref string) (*api.Service, error) {
	// Refuse the blanket case before resolving, so read_only=all costs no
	// network call.
	if cfg.ReadOnly.BlocksAll() {
		return nil, common.ErrReadOnly
	}

	service, err := resolveService(ctx, client, projectID, ref)
	if err != nil {
		return nil, err
	}

	// Resolving returns the whole service, so prod's half of the gate needs
	// no second fetch.
	if err := common.CheckReadOnlyService(cfg, *service); err != nil {
		return nil, err
	}

	return service, nil
}

// resolveServiceID is resolveService for the commands that pass the ID to
// another endpoint and never need the service itself.
func resolveServiceID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, ref string) (string, error) {
	service, err := resolveService(ctx, client, projectID, ref)
	if err != nil {
		return "", err
	}
	return service.ServiceID, nil
}
