package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func setupBillingTest(t *testing.T) string {
	t.Helper()

	// Use a unique keyring service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	tmpDir, err := os.MkdirTemp("", "tiger-billing-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_ANALYTICS", "false")

	config.ResetGlobalConfig()

	t.Cleanup(func() {
		config.ResetGlobalConfig()
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeBillingCommand(ctx context.Context, args ...string) (string, error, *cobra.Command) {
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err, nil
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err, testRoot
}

func TestBillingDraftInvoice_JSON(t *testing.T) {
	tmpDir := setupBillingTest(t)

	projectID := "test-project-789"

	// Mock server that serves the draft-invoice endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/projects/" + projectID + "/billing/draft-invoice"
		if r.URL.Path != expectedPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		serviceID := "svc-1"
		label := "Compute"
		amount := "15.00"
		invoice := api.DraftInvoice{
			BillingAccountId: strPtr("42"),
			Currency:         strPtr("USD"),
			Status:           strPtr("draft"),
			AmountDue:        strPtr("15.00"),
			Subtotal:         strPtr("15.00"),
			Total:            strPtr("15.00"),
			LineItems: &[]api.DraftInvoiceLineItem{
				{ServiceId: &serviceID, Label: &label, Amount: &amount},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(invoice)
	}))
	defer mockServer.Close()

	// Point config at the mock server
	configFile := config.GetConfigFile(tmpDir)
	configContent := "api_url: \"" + mockServer.URL + "\"\n"
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Store credentials so the project ID is available without /auth/info
	if err := config.StoreCredentials("test-api-key-789", projectID); err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	output, err, _ := executeBillingCommand(t.Context(), "billing", "draft-invoice", "--output", "json")
	if err != nil {
		t.Fatalf("draft-invoice failed: %v", err)
	}

	var result api.DraftInvoice
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Output should be valid JSON: %v\noutput: %s", err, output)
	}

	if result.Status == nil || *result.Status != "draft" {
		t.Errorf("Expected status 'draft', got: %v", result.Status)
	}

	if !strings.Contains(output, "Compute") {
		t.Errorf("Expected output to contain line item label 'Compute', got: %s", output)
	}
}

func strPtr(s string) *string {
	return &s
}
