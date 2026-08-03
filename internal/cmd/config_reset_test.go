package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestConfigReset(t *testing.T) {
	_, _ = setupConfigTest(t)

	// First set some custom values
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	cfg.Set("service_id", "custom-service")
	cfg.Set("output", "json")
	cfg.Set("analytics", "false")

	// Execute reset command
	output, err := executeConfigCommand(t.Context(), "config", "reset")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if !strings.Contains(output, "Configuration reset to defaults") {
		t.Errorf("Expected output to contain reset message, got '%s'", strings.TrimSpace(output))
	}

	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify all values were reset to defaults
	if cfg.APIURL != config.DefaultAPIURL {
		t.Errorf("Expected default APIURL %s, got %s", config.DefaultAPIURL, cfg.APIURL)
	}
	if cfg.ServiceID != "" {
		t.Errorf("Expected empty ServiceID, got %s", cfg.ServiceID)
	}
	if cfg.Output != config.DefaultOutput {
		t.Errorf("Expected default Output %s, got %s", config.DefaultOutput, cfg.Output)
	}
	if cfg.Analytics != config.DefaultAnalytics {
		t.Errorf("Expected default Analytics %t, got %t", config.DefaultAnalytics, cfg.Analytics)
	}
}
