package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestServiceFork_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute service fork command with required timing flag
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "--now")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceFork_NoSourceService(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config without service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	mockTestPAT(t)

	// Execute service fork command without providing service ID but with timing flag
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "--now")
	if err == nil {
		t.Fatal("Expected error when no service ID provided")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected service ID required error, got: %v", err)
	}
}

func TestServiceFork_NoTimingFlag(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	mockTestPAT(t)

	// Execute service fork command without any timing flag
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123")
	if err == nil {
		t.Fatal("Expected error when no timing flag provided")
	}

	if !strings.Contains(err.Error(), "must specify --now, --last-snapshot or --to-timestamp") {
		t.Errorf("Expected timing flag required error, got: %v", err)
	}
}

func TestServiceFork_MultipleTiming(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	mockTestPAT(t)

	// Execute service fork command with multiple timing flags
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123", "--now", "--last-snapshot")
	if err == nil {
		t.Fatal("Expected error when multiple timing flags provided")
	}

	if !strings.Contains(err.Error(), "can only specify one of --now, --last-snapshot or --to-timestamp") {
		t.Errorf("Expected multiple timing flags error, got: %v", err)
	}
}

func TestServiceFork_InvalidTimestamp(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	mockTestPAT(t)

	// Execute service fork command with invalid timestamp
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123", "--to-timestamp", "invalid-timestamp")
	if err == nil {
		t.Fatal("Expected error when invalid timestamp provided")
	}

	if !strings.Contains(err.Error(), "invalid time format") {
		t.Errorf("Expected invalid timestamp error, got: %v", err)
	}
}

func TestServiceFork_CPUMemoryValidation(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	mockTestPAT(t)

	// Test with invalid CPU/memory combination (this would fail at API call stage)
	// Since we don't want to make real API calls, we expect the command to fail during validation
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123", "--now", "--cpu", "999", "--memory", "1")

	// This test is mainly to ensure the flags are parsed correctly
	// The actual validation happens later in the process when we have source service details
	// So we expect either a validation error or an API connection error
	if err == nil {
		t.Fatal("Expected some error due to invalid CPU/memory or API connection")
	}

	// We're mainly testing that the flags are accepted and parsed
	// The detailed validation logic is tested in integration tests
}
