package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestServiceResize_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with API URL
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute service resize command
	_, _, err = executeServiceCommand(t.Context(), "service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}

	// Check for proper exit code
	if exitErr, ok := err.(interface{ ExitCode() int }); !ok || exitErr.ExitCode() != common.ExitAuthenticationError {
		t.Errorf("Expected exit code %d, got: %v", common.ExitAuthenticationError, err)
	}
}

func TestServiceResize_MissingParams(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with API URL
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Test missing both CPU and memory parameters
	_, _, err = executeServiceCommand(t.Context(), "service", "resize")
	if err == nil {
		t.Fatal("Expected error when CPU and memory are missing")
	}

	if !strings.Contains(err.Error(), "must specify at least one of --cpu or --memory") {
		t.Errorf("Expected missing params error, got: %v", err)
	}
}

func TestServiceResize_InvalidCPUMemoryCombination(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with API URL
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Test invalid CPU/memory combination
	_, _, err = executeServiceCommand(t.Context(), "service", "resize", "--cpu", "3000", "--memory", "8")
	if err == nil {
		t.Fatal("Expected error for invalid CPU/memory combination")
	}

	if !strings.Contains(err.Error(), "invalid CPU/Memory combination") {
		t.Errorf("Expected invalid combination error, got: %v", err)
	}
}
