package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func setupBillingToolTest(t *testing.T) string {
	t.Helper()

	// Use a unique keyring service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	tmpDir, err := os.MkdirTemp("", "tiger-mcp-billing-test-*")
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

func TestBillingGetDraftInvoice(t *testing.T) {
	tmpDir := setupBillingToolTest(t)

	projectID := "test-project-789"

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
			BillingAccountId: ptr("42"),
			Currency:         ptr("USD"),
			Status:           ptr("draft"),
			AmountDue:        ptr("15.00"),
			Subtotal:         ptr("15.00"),
			Total:            ptr("15.00"),
			LineItems: &[]api.DraftInvoiceLineItem{
				{ServiceId: &serviceID, Label: &label, Amount: &amount},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(invoice)
	}))
	defer mockServer.Close()

	if _, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": mockServer.URL,
	}); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	if err := config.StoreCredentials("test-api-key-789", projectID); err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	s := &Server{}
	_, output, err := s.handleBillingGetDraftInvoice(t.Context(), nil, BillingGetDraftInvoiceInput{})
	if err != nil {
		t.Fatalf("handleBillingGetDraftInvoice failed: %v", err)
	}

	if output.Invoice.Status != "draft" {
		t.Errorf("Expected status 'draft', got: %q", output.Invoice.Status)
	}
	if output.Invoice.Currency != "USD" {
		t.Errorf("Expected currency 'USD', got: %q", output.Invoice.Currency)
	}
	if output.Invoice.Total != "15.00" {
		t.Errorf("Expected total '15.00', got: %q", output.Invoice.Total)
	}
	if len(output.Invoice.LineItems) != 1 {
		t.Fatalf("Expected 1 line item, got: %d", len(output.Invoice.LineItems))
	}
	if output.Invoice.LineItems[0].Label != "Compute" {
		t.Errorf("Expected line item label 'Compute', got: %q", output.Invoice.LineItems[0].Label)
	}
}

func ptr(s string) *string {
	return &s
}
