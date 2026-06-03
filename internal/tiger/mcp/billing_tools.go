package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// MCP tool name for billing.
const (
	toolBillingGetDraftInvoice = "billing_get_draft_invoice"
)

// BillingGetDraftInvoiceInput represents input for billing_get_draft_invoice.
// It takes no parameters and operates on the current project.
type BillingGetDraftInvoiceInput struct{}

func (BillingGetDraftInvoiceInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[BillingGetDraftInvoiceInput](nil))
}

// BillingGetDraftInvoiceOutput represents output for billing_get_draft_invoice.
type BillingGetDraftInvoiceOutput struct {
	Invoice DraftInvoiceDetail `json:"invoice"`
}

func (BillingGetDraftInvoiceOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[BillingGetDraftInvoiceOutput](nil))
}

// DraftInvoiceDetail represents the current draft invoice for the project.
type DraftInvoiceDetail struct {
	BillingAccountID   string                       `json:"billing_account_id,omitempty"`
	Currency           string                       `json:"currency,omitempty"`
	Status             string                       `json:"status" jsonschema:"Invoice status; \"none\" when nothing is due"`
	BillingPeriodStart string                       `json:"billing_period_start,omitempty" jsonschema:"Start of the billing period (RFC3339)"`
	BillingPeriodEnd   string                       `json:"billing_period_end,omitempty" jsonschema:"End of the billing period (RFC3339)"`
	AmountDue          string                       `json:"amount_due,omitempty"`
	Subtotal           string                       `json:"subtotal,omitempty"`
	Total              string                       `json:"total,omitempty"`
	LineItems          []DraftInvoiceLineItemDetail `json:"line_items,omitempty"`
}

func (DraftInvoiceDetail) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[DraftInvoiceDetail](nil))
}

// DraftInvoiceLineItemDetail represents a per-service line item on the draft invoice.
type DraftInvoiceLineItemDetail struct {
	ServiceID string `json:"service_id,omitempty"`
	Label     string `json:"label,omitempty"`
	Amount    string `json:"amount,omitempty"`
}

// registerBillingTools registers billing tools with comprehensive schemas and descriptions
func (s *Server) registerBillingTools() {
	// billing_get_draft_invoice
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  toolBillingGetDraftInvoice,
		Title: "Get Draft Invoice",
		Description: "Get the current draft (upcoming) invoice for the current Tiger Cloud project. " +
			"Returns billing period, totals, amount due, and a per-service cost breakdown. " +
			"Status is \"none\" with zeroed totals when nothing is due yet.",
		InputSchema:  BillingGetDraftInvoiceInput{}.Schema(),
		OutputSchema: BillingGetDraftInvoiceOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(true),
			Title:         "Get Draft Invoice",
		},
	}, s.handleBillingGetDraftInvoice)
}

// handleBillingGetDraftInvoice handles the billing_get_draft_invoice MCP tool
func (s *Server) handleBillingGetDraftInvoice(ctx context.Context, req *mcp.CallToolRequest, input BillingGetDraftInvoiceInput) (*mcp.CallToolResult, BillingGetDraftInvoiceOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, BillingGetDraftInvoiceOutput{}, err
	}

	logging.Debug("MCP: Getting draft invoice",
		zap.String("project_id", cfg.ProjectID))

	// Make API call to get the draft invoice
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetDraftInvoiceWithResponse(ctx, cfg.ProjectID)
	if err != nil {
		return nil, BillingGetDraftInvoiceOutput{}, fmt.Errorf("failed to get draft invoice: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusOK {
		return nil, BillingGetDraftInvoiceOutput{}, resp.JSON4XX
	}

	if resp.JSON200 == nil {
		return nil, BillingGetDraftInvoiceOutput{}, fmt.Errorf("empty response from API")
	}

	output := BillingGetDraftInvoiceOutput{
		Invoice: convertToDraftInvoiceDetail(*resp.JSON200),
	}

	return nil, output, nil
}

// convertToDraftInvoiceDetail converts an api.DraftInvoice into the MCP output shape
func convertToDraftInvoiceDetail(invoice api.DraftInvoice) DraftInvoiceDetail {
	detail := DraftInvoiceDetail{
		BillingAccountID: util.Deref(invoice.BillingAccountId),
		Currency:         util.Deref(invoice.Currency),
		Status:           util.Deref(invoice.Status),
		AmountDue:        util.Deref(invoice.AmountDue),
		Subtotal:         util.Deref(invoice.Subtotal),
		Total:            util.Deref(invoice.Total),
	}

	if invoice.BillingPeriodStart != nil {
		detail.BillingPeriodStart = invoice.BillingPeriodStart.Format(time.RFC3339)
	}
	if invoice.BillingPeriodEnd != nil {
		detail.BillingPeriodEnd = invoice.BillingPeriodEnd.Format(time.RFC3339)
	}

	if invoice.LineItems != nil {
		detail.LineItems = make([]DraftInvoiceLineItemDetail, 0, len(*invoice.LineItems))
		for _, item := range *invoice.LineItems {
			detail.LineItems = append(detail.LineItems, DraftInvoiceLineItemDetail{
				ServiceID: util.Deref(item.ServiceId),
				Label:     util.Deref(item.Label),
				Amount:    util.Deref(item.Amount),
			})
		}
	}

	return detail
}
