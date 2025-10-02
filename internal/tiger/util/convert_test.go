package util

import (
	"testing"
)

// Define local types for testing to avoid import cycles
type testDeployStatus string
type testServiceType string

func TestDeref(t *testing.T) {
	// Test service ID formatting (now uses string instead of UUID)
	testServiceID := "12345678-9abc-def0-1234-56789abcdef0"
	if Deref(&testServiceID) != testServiceID {
		t.Error("Deref should return service ID string")
	}
	if Deref((*string)(nil)) != "" {
		t.Error("Deref should return empty string for nil")
	}

	// Test Deref with string
	testStr := "test"
	if Deref(&testStr) != "test" {
		t.Error("Deref should return string value")
	}
	if Deref((*string)(nil)) != "" {
		t.Error("Deref should return empty string for nil")
	}
}

func TestDerefStr(t *testing.T) {
	// Test DerefStr with DeployStatus
	status := testDeployStatus("running")
	if DerefStr(&status) != "running" {
		t.Error("DerefStr should return status string")
	}
	if DerefStr((*testDeployStatus)(nil)) != "" {
		t.Error("DerefStr should return empty string for nil")
	}

	// Test DerefStr with ServiceType
	serviceType := testServiceType("POSTGRES")
	if DerefStr(&serviceType) != "POSTGRES" {
		t.Error("DerefStr should return service type string")
	}
	if DerefStr((*testServiceType)(nil)) != "" {
		t.Error("DerefStr should return empty string for nil")
	}
}
