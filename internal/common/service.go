package common

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/util"
)

// Matches front-end logic for generating a random service name
func GenerateServiceName() string {
	return fmt.Sprintf("db-%d", 10000+rand.Intn(90000))
}

// ServiceEnvironmentTag returns a service's environment tag. A fetched service
// carries it as a free-form string under metadata, not the enum the create and
// fork requests take. Only PROD is recognized: absent, empty and unknown values
// all read as DEV, because metadata is optional in the response and treating
// unknowns as protected would have read_only=prod block untagged services.
func ServiceEnvironmentTag(service api.Service) api.EnvironmentTag {
	if service.Metadata == nil {
		return api.EnvironmentTagDEV
	}

	if strings.EqualFold(util.DerefStr(service.Metadata.Environment), string(api.EnvironmentTagPROD)) {
		return api.EnvironmentTagPROD
	}
	return api.EnvironmentTagDEV
}

// Addon constants - these match the ServiceCreateAddons from the API
const (
	AddonNone       = "none" // Special value for no add-ons
	AddonTimeSeries = "time-series"
	AddonAI         = "ai"
)

// ValidAddons returns a slice of all valid add-on values
func ValidAddons() []string {
	return []string{
		AddonTimeSeries,
		AddonAI,
	}
}

// IsValidAddon checks if the given add-on is valid (case-sensitive as per API spec)
func IsValidAddon(addon string) bool {
	for _, validAddon := range ValidAddons() {
		if addon == validAddon {
			return true
		}
	}
	return false
}

// ValidateAddons validates a slice of add-ons and removes duplicate values
func ValidateAddons(addons []string) ([]string, error) {
	if len(addons) == 0 {
		return nil, nil
	}

	// Check if first element is "none" - if so, return empty list (no add-ons)
	if len(addons) == 1 && strings.ToLower(addons[0]) == AddonNone {
		return []string{}, nil
	}

	var (
		seen   = make(map[string]bool)
		result []string
	)
	for _, addon := range addons {
		addon = strings.TrimSpace(addon)

		if !IsValidAddon(addon) {
			return nil, fmt.Errorf("invalid add-on '%s'. Valid add-ons: %s, or 'none' for PostgreSQL-only", addon, strings.Join(ValidAddons(), ", "))
		}
		if seen[addon] {
			continue
		}
		seen[addon] = true
		result = append(result, addon)
	}

	return result, nil
}
