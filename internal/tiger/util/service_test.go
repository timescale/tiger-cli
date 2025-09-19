package util

import (
	"reflect"
	"testing"
)

func TestValidServiceTypes(t *testing.T) {
	expected := []string{"timescaledb", "postgres", "vector"}
	result := ValidServiceTypes()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ValidServiceTypes() = %v, want %v", result, expected)
	}
}

func TestIsValidServiceType(t *testing.T) {
	tests := []struct {
		name        string
		serviceType string
		want        bool
	}{
		{"Valid timescaledb", "timescaledb", true},
		{"Valid postgres", "postgres", true},
		{"Valid vector", "vector", true},
		{"Valid timescaledb uppercase", "TIMESCALEDB", true},
		{"Valid postgres mixed case", "PostGres", true},
		{"Invalid service type", "invalid", false},
		{"Empty string", "", false},
		{"Similar but wrong", "postgresql", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidServiceType(tt.serviceType); got != tt.want {
				t.Errorf("IsValidServiceType(%q) = %v, want %v", tt.serviceType, got, tt.want)
			}
		})
	}
}

func TestServiceTypeConstants(t *testing.T) {
	// Verify constants have expected values
	if ServiceTypeTimescaleDB != "timescaledb" {
		t.Errorf("ServiceTypeTimescaleDB = %q, want %q", ServiceTypeTimescaleDB, "timescaledb")
	}
	if ServiceTypePostgres != "postgres" {
		t.Errorf("ServiceTypePostgres = %q, want %q", ServiceTypePostgres, "postgres")
	}
	if ServiceTypeVector != "vector" {
		t.Errorf("ServiceTypeVector = %q, want %q", ServiceTypeVector, "vector")
	}
}
