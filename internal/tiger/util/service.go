package util

import (
	"fmt"
	"math/rand"
	"strings"
)

// Matches front-end logic for generating a random service name
func GenerateServiceName() string {
	return fmt.Sprintf("db-%d", 10000+rand.Intn(90000))
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
			return nil, fmt.Errorf("invalid add-on '%s'. Valid add-ons: %s, or 'auto' for automatic selection, or 'none' for no add-ons", addon, strings.Join(ValidAddons(), ", "))
		}
		if seen[addon] {
			continue
		}
		seen[addon] = true
		result = append(result, addon)
	}

	return result, nil
}
