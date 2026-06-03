package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// buildBillingCmd creates the main billing command with all subcommands
func buildBillingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "billing",
		Aliases: []string{"invoice"},
		Short:   "View billing information",
		Long:    `View billing information for the current Tiger Cloud project.`,
	}

	cmd.AddCommand(buildBillingDraftInvoiceCmd())

	return cmd
}

// buildBillingDraftInvoiceCmd represents the draft-invoice command under billing
func buildBillingDraftInvoiceCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "draft-invoice",
		Aliases: []string{"current-invoice"},
		Short:   "Show the current draft invoice",
		Long: `Show the current draft (upcoming) invoice for the current project,
including a per-service cost breakdown.

Examples:
  # Show the current draft invoice
  tiger billing draft-invoice

  # Show the draft invoice in JSON format
  tiger billing draft-invoice --output json

  # Show the draft invoice in YAML format
  tiger billing draft-invoice --output yaml`,
		Args:    cobra.NoArgs,
		PreRunE: bindFlags("output"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Load config and API client
			cfg, err := common.LoadConfig(cmd.Context())
			if err != nil {
				return err
			}

			// Make API call to get the draft invoice
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := cfg.Client.GetDraftInvoiceWithResponse(ctx, cfg.ProjectID)
			if err != nil {
				return fmt.Errorf("failed to get draft invoice: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			// Output draft invoice in requested format
			return outputDraftInvoice(cmd, *resp.JSON200, cfg.Output)
		},
	}

	cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "Output format (json, yaml, table)")

	return cmd
}

// outputDraftInvoice formats and outputs the draft invoice based on the specified format
func outputDraftInvoice(cmd *cobra.Command, invoice api.DraftInvoice, format string) error {
	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, invoice)
	case "yaml":
		return util.SerializeToYAML(outputWriter, invoice)
	default: // table format (default)
		return outputDraftInvoiceTable(invoice, outputWriter)
	}
}

// outputDraftInvoiceTable outputs detailed draft invoice information in formatted tables
func outputDraftInvoiceTable(invoice api.DraftInvoice, output io.Writer) error {
	// Summary table
	summary := tablewriter.NewWriter(output)
	summary.Header("PROPERTY", "VALUE")

	summary.Append("Status", util.Deref(invoice.Status))
	summary.Append("Currency", util.Deref(invoice.Currency))
	if invoice.BillingPeriodStart != nil {
		summary.Append("Period Start", invoice.BillingPeriodStart.Format("2006-01-02"))
	}
	if invoice.BillingPeriodEnd != nil {
		summary.Append("Period End", invoice.BillingPeriodEnd.Format("2006-01-02"))
	}
	summary.Append("Subtotal", util.Deref(invoice.Subtotal))
	summary.Append("Total", util.Deref(invoice.Total))
	summary.Append("Amount Due", util.Deref(invoice.AmountDue))

	if err := summary.Render(); err != nil {
		return err
	}

	// Per-service line items table
	if invoice.LineItems != nil && len(*invoice.LineItems) > 0 {
		items := tablewriter.NewWriter(output)
		items.Header("SERVICE ID", "PRODUCT", "AMOUNT")

		for _, item := range *invoice.LineItems {
			items.Append(
				util.Deref(item.ServiceId),
				util.Deref(item.Label),
				util.Deref(item.Amount),
			)
		}

		if err := items.Render(); err != nil {
			return err
		}
	}

	return nil
}
