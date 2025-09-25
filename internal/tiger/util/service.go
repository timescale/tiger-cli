package util

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

// Matches front-end logic for generating a random service name
func GenerateServiceName() string {
	return fmt.Sprintf("db-%d", 10000+rand.Intn(90000))
}

// Addon constants - these match the ServiceCreateAddons from the API
const (
	AddonTimeSeries = "time-series"
	AddonAI         = "ai"
	AddonNone       = "none" // Special value for no add-ons
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

// ValidateAddons validates a slice of add-ons, handling both individual add-ons
// and comma-separated values within individual elements (for pflag StringSlice compatibility)
// Returns a flattened, validated slice or error if any add-on is invalid
func ValidateAddons(addons []string) ([]string, error) {
	if len(addons) == 0 {
		return nil, nil
	}

	// Check if first element is "none" - if so, return nil (no add-ons)
	if len(addons) == 1 && strings.ToLower(addons[0]) == AddonNone {
		return nil, nil
	}

	var (
		seen   = make(map[string]bool)
		result []string
	)
	for _, addon := range addons {
		addon = strings.TrimSpace(addon)

		if !IsValidAddon(addon) {
			return nil, fmt.Errorf("invalid add-on '%s'. Valid add-ons: %s, or 'none' for no add-ons", addon, strings.Join(ValidAddons(), ", "))
		}
		if seen[addon] {
			continue
		}
		seen[addon] = true
		result = append(result, addon)
	}

	return result, nil
}

// ConvertAddonsToAPI converts a slice of addon strings to the API format
// Returns nil if addons slice is empty or nil (for PostgreSQL-only services)
func ConvertAddonsToAPI(addons []string) *[]api.ServiceCreateAddons {
	if len(addons) == 0 {
		return nil
	}

	apiAddons := make([]api.ServiceCreateAddons, len(addons))
	for i, addon := range addons {
		apiAddons[i] = api.ServiceCreateAddons(addon)
	}
	return &apiAddons
}
