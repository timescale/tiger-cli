package util

import (
	"fmt"
	"math/rand"
	"strings"
)

// Service type constants
const (
	ServiceTypeTimescaleDB = "timescaledb"
	ServiceTypePostgres    = "postgres"
	ServiceTypeVector      = "vector"
)

// ValidServiceTypes returns a slice of all valid service types
func ValidServiceTypes() []string {
	return []string{
		ServiceTypeTimescaleDB,
		ServiceTypePostgres,
		ServiceTypeVector,
	}
}

// IsValidServiceType checks if the given service type is valid (case-insensitive)
func IsValidServiceType(serviceType string) bool {
	lowerType := strings.ToLower(serviceType)
	for _, validType := range ValidServiceTypes() {
		if lowerType == validType {
			return true
		}
	}
	return false
}

// Matches front-end logic for generating a random service name
func GenerateServiceName() string {
	return fmt.Sprintf("db-%d", 10000+rand.Intn(90000))
}
