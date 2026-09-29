package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// errServiceRequired is returned when neither an argument nor a configured
// default identifies a service.
var errServiceRequired = errors.New("service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")

// serviceRef is what the user gave to identify a service, and where it came
// from. The two resolve differently: an argument may be an ID, a read replica
// set ID, or a name, and the API matches all of them together, while a
// configured default must be an ID. A stored name would stop working the
// moment someone renamed the service, and unlike an argument typed fresh each
// time, nobody is watching when it does.
type serviceRef struct {
	ref string

	// source names where a configured default came from, so an error can say
	// which one to fix. It is empty for a ref given as an argument, which is
	// also what marks that ref as free to be a name.
	source string
}

// argServiceRef is a ref a command read straight out of its arguments, for the
// commands that take no default.
func argServiceRef(arg string) serviceRef {
	return serviceRef{ref: arg}
}

// getServiceRef determines the service ref from args or the configured default.
func getServiceRef(cmd *cobra.Command, cfg *config.Config, args []string) (serviceRef, error) {
	if len(args) > 0 {
		if args[0] == "" {
			return serviceRef{}, errServiceRequired
		}
		return argServiceRef(args[0]), nil
	}

	if cfg.ServiceID == "" {
		return serviceRef{}, errServiceRequired
	}
	return serviceRef{ref: cfg.ServiceID, source: defaultServiceSource(cmd)}, nil
}

// defaultServiceSource names the layer the default service came from, in the
// same precedence config.Load uses, so the error names the one to fix.
func defaultServiceSource(cmd *cobra.Command) string {
	if flag := cmd.Flags().Lookup("service-id"); flag != nil && flag.Changed {
		return "--service-id"
	}
	if os.Getenv("TIGER_SERVICE_ID") != "" {
		return "TIGER_SERVICE_ID"
	}
	return "the service_id config value"
}

// resolveService resolves a ref to the service it names, through the ref
// filter on the service list: a match is a one-item list, and no match is an
// empty one rather than a 404.
func resolveService(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, ref serviceRef) (*api.Service, error) {
	resp, err := client.GetServicesWithResponse(ctx, projectID, &api.GetServicesParams{Ref: &ref.ref})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve service '%s': %w", ref.ref, err)
	}
	if resp.StatusCode() != http.StatusOK {
		err := common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
		// The backend's only validation error on this path is the refusal of
		// a ref matching more than one service, so a 400 here means that.
		// The server's message doesn't name the candidates, so point at the
		// command that lists them.
		if resp.StatusCode() == http.StatusBadRequest {
			return nil, fmt.Errorf("%w\nRun 'tiger service list' to find the ID you want", err)
		}
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	services := *resp.JSON200
	if len(services) == 0 {
		return nil, common.ExitWithCode(common.ExitServiceNotFound, fmt.Errorf("service '%s' not found", ref.ref))
	}
	// The API refuses an ambiguous ref with a 400 instead, so this means the
	// filter was ignored; picking one would act on the wrong service.
	if len(services) > 1 {
		return nil, fmt.Errorf("expected one service for '%s', got %d", ref.ref, len(services))
	}

	service := services[0]
	if err := checkDefaultIsID(service, ref); err != nil {
		return nil, err
	}
	return &service, nil
}

// checkDefaultIsID refuses a configured default that turned out to be a name
// (see serviceRef for why). A ref equal to the service's own ID matched by ID
// or replica set ID; anything else matched by name. Checking after the lookup
// rather than before is what lets the error name the ID to store instead.
func checkDefaultIsID(service api.Service, ref serviceRef) error {
	if ref.source == "" || service.ServiceID == ref.ref {
		return nil
	}
	return common.ExitWithCode(common.ExitInvalidParameters, fmt.Errorf(
		"the default service must be an ID, but %s is set to the name '%s'. Set it to %s, or pass the name as an argument",
		ref.source, ref.ref, service.ServiceID))
}

// serviceLabel identifies a service in status output by name and ID together.
// A user who typed a name gets to see which service it resolved to, and one
// who typed an ID still gets the name that goes with it.
func serviceLabel(service api.Service) string {
	return fmt.Sprintf("'%s' (%s)", service.Name, service.ServiceID)
}

// resolveServiceForWrite is resolveService for a command that changes the
// service it resolves. It refuses the blanket read-only case before the
// network call and gates on the tag the resolution returns, so neither half
// of the gate can be skipped or put in the wrong order at a call site.
//
// Commands that only read a service use resolveService. So does service fork:
// it resolves a source but gates on the environment it is about to request,
// not on the source's.
func resolveServiceForWrite(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface, projectID string, ref serviceRef) (*api.Service, error) {
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
	if err := common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(*service)); err != nil {
		return nil, err
	}

	return service, nil
}
