package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// buildFileImportCmd creates the main file-import command with all subcommands
func buildFileImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "file-import",
		Aliases: []string{"fileimport", "import"},
		Short:   "Manage file imports",
		Long:    `Import CSV and Parquet files into database tables.`,
	}

	// Add all subcommands
	cmd.AddCommand(buildFileImportUploadCmd())
	cmd.AddCommand(buildFileImportListCmd())
	cmd.AddCommand(buildFileImportGetCmd())
	cmd.AddCommand(buildFileImportCancelCmd())

	return cmd
}

// buildFileImportUploadCmd creates the upload command
func buildFileImportUploadCmd() *cobra.Command {
	var table string
	var schema string
	var skipHeader bool
	var delimiter string
	var autoColumnMapping bool
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload and import a file",
		Long: `Upload a CSV or Parquet file and import it into a database table.

This command performs the complete workflow:
1. Generates a presigned S3 URL
2. Uploads the file to S3
3. Creates a file import operation
4. Waits for the import to complete (by default)

Examples:
  # Upload CSV file to table 'sales_data'
  tiger file-import upload data.csv --table sales_data

  # Upload with custom schema and skip header
  tiger file-import upload data.csv --table orders --schema analytics --skip-header

  # Upload without waiting for completion
  tiger file-import upload large.csv --table big_table --no-wait`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			// Validate required flags
			if table == "" {
				return fmt.Errorf("--table flag is required")
			}

			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Get credentials
			apiKey, projectID, err := getCredentialsForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
			}

			serviceID := cfg.ServiceID
			if serviceID == "" {
				return fmt.Errorf("service ID is required. Set it with 'tiger config set service_id <service-id>'")
			}

			// Get file info
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			contentLength := int(fileInfo.Size())
			if contentLength > 524288000 {
				return fmt.Errorf("file size exceeds 500MB limit")
			}

			// Create API client
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Generate unique ID for this upload
			importID := fmt.Sprintf("cli-upload-%d", time.Now().Unix())

			// Step 1: Generate presigned URL
			fmt.Fprintf(cmd.OutOrStdout(), "Generating upload URL...\n")
			presignedResp, err := client.PostProjectsProjectIdServicesServiceIdFileimportsPresignedUrlWithResponse(
				ctx,
				projectID,
				serviceID,
				api.PostProjectsProjectIdServicesServiceIdFileimportsPresignedUrlJSONRequestBody{
					Id:            importID,
					ContentLength: contentLength,
				},
			)
			if err != nil {
				return fmt.Errorf("failed to generate presigned URL: %w", err)
			}

			if presignedResp.StatusCode() != 200 || presignedResp.JSON200 == nil {
				return exitWithErrorFromStatusCode(presignedResp.StatusCode(), presignedResp.JSON4XX)
			}

			presignedURL := presignedResp.JSON200.Url

			// Step 2: Upload file to S3
			fmt.Fprintf(cmd.OutOrStdout(), "Uploading file (%d bytes)...\n", contentLength)
			file, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer file.Close()

			if err := uploadFileToS3(presignedURL, file, contentLength); err != nil {
				return fmt.Errorf("failed to upload file: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Upload successful\n")

			// Step 3: Create file import
			fmt.Fprintf(cmd.OutOrStdout(), "Creating file import...\n")

			// Determine file type from extension
			fileType := "CSV"
			if strings.HasSuffix(strings.ToLower(filePath), ".parquet") {
				fileType = "PARQUET"
			}

			// Build definition based on file type
			var definition api.FileImportDefinition
			definition.Type = api.FileImportDefinitionType(fileType)

			if fileType == "CSV" {
				definition.Csv = &api.FileImportDefinitionCSV{
					Delimiter:         &delimiter,
					SkipHeader:        &skipHeader,
					AutoColumnMapping: &autoColumnMapping,
				}
			} else {
				definition.Parquet = &api.FileImportDefinitionParquet{
					AutoColumnMapping: &autoColumnMapping,
				}
			}

			tableIdentifier := api.TableIdentifier{
				TableName: table,
			}
			if schema != "" {
				tableIdentifier.SchemaName = &schema
			}

			createResp, err := client.PostProjectsProjectIdServicesServiceIdFileimportsWithResponse(
				ctx,
				projectID,
				serviceID,
				api.PostProjectsProjectIdServicesServiceIdFileimportsJSONRequestBody{
					Id: importID,
					Source: api.FileImportSource{
						Type: api.FileImportSourceTypeINTERNAL,
						Internal: &api.FileImportInternalSource{
							Id: importID,
						},
					},
					Definition:      definition,
					TableIdentifier: tableIdentifier,
					Labels: &[]api.FileImportLabel{
						{Key: "source", Value: "tiger-cli"},
						{Key: "file_name", Value: fileInfo.Name()},
					},
				},
			)
			if err != nil {
				return fmt.Errorf("failed to create file import: %w", err)
			}

			if createResp.StatusCode() != 201 {
				return exitWithErrorFromStatusCode(createResp.StatusCode(), createResp.JSON4XX)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "File import created: %s\n", importID)

			// Step 4: Wait for completion (unless --no-wait)
			if !noWait {
				fmt.Fprintf(cmd.OutOrStdout(), "Waiting for import to complete...\n")
				if err := waitForImportCompletion(ctx, client, projectID, serviceID, importID, waitTimeout, cmd.OutOrStdout()); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Import started. Use 'tiger file-import get %s' to check status\n", importID)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&table, "table", "", "destination table name (required)")
	cmd.Flags().StringVar(&schema, "schema", "", "destination schema name (defaults to 'public')")
	cmd.Flags().BoolVar(&skipHeader, "skip-header", true, "skip first row as header")
	cmd.Flags().StringVar(&delimiter, "delimiter", ",", "CSV delimiter character")
	cmd.Flags().BoolVar(&autoColumnMapping, "auto-column-mapping", true, "automatically map columns by name")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "don't wait for import to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Minute, "maximum time to wait for import completion")

	cmd.MarkFlagRequired("table")

	return cmd
}

// buildFileImportListCmd creates the list command
func buildFileImportListCmd() *cobra.Command {
	var output string
	var first int
	var states []string
	var labelSelector string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List file imports",
		Long: `List file imports for the current service with optional filtering.

Examples:
  # List recent imports
  tiger file-import list

  # List first 20 imports
  tiger file-import list --first 20

  # List only successful imports
  tiger file-import list --states SUCCESS

  # List failed and cancelled imports
  tiger file-import list --states FAILURE,CANCELLED

  # List imports with specific label
  tiger file-import list --label-selector source=tiger-cli`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cmd.Flags().Changed("output") {
				cfg.Output = output
			}

			cmd.SilenceUsage = true

			// Get credentials
			apiKey, projectID, err := getCredentialsForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
			}

			serviceID := cfg.ServiceID
			if serviceID == "" {
				return fmt.Errorf("service ID is required. Set it with 'tiger config set service_id <service-id>'")
			}

			// Create API client
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Build query params
			params := &api.GetProjectsProjectIdServicesServiceIdFileimportsParams{
				First: &first,
			}

			if len(states) > 0 {
				statesStr := strings.Join(states, ",")
				params.States = &statesStr
			}

			if labelSelector != "" {
				params.LabelSelector = &labelSelector
			}

			// Make API call
			resp, err := client.GetProjectsProjectIdServicesServiceIdFileimportsWithResponse(ctx, projectID, serviceID, params)
			if err != nil {
				return fmt.Errorf("failed to list file imports: %w", err)
			}

			if resp.StatusCode() != 200 {
				return exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			// Output file imports
			return outputFileImportList(cmd, resp.JSON200.FileImports, cfg.Output)
		},
	}

	cmd.Flags().VarP((*outputWithEnvFlag)(&output), "output", "o", "output format (json, yaml, table)")
	cmd.Flags().IntVar(&first, "first", 10, "number of imports to fetch")
	cmd.Flags().StringSliceVar(&states, "states", nil, "filter by states (e.g., SUCCESS,FAILURE)")
	cmd.Flags().StringVar(&labelSelector, "label-selector", "", "filter by labels (e.g., source=tiger-cli)")

	return cmd
}

// buildFileImportGetCmd creates the get command
func buildFileImportGetCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "get <import-id>",
		Aliases: []string{"show", "describe"},
		Short:   "Get file import details",
		Long: `Get detailed information about a specific file import.

Examples:
  # Get import details
  tiger file-import get cli-upload-1234567890

  # Get details in JSON format
  tiger file-import get cli-upload-1234567890 --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			importID := args[0]

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cmd.Flags().Changed("output") {
				cfg.Output = output
			}

			cmd.SilenceUsage = true

			// Get credentials
			apiKey, projectID, err := getCredentialsForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
			}

			serviceID := cfg.ServiceID
			if serviceID == "" {
				return fmt.Errorf("service ID is required. Set it with 'tiger config set service_id <service-id>'")
			}

			// Create API client
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Make API call
			resp, err := client.GetProjectsProjectIdServicesServiceIdFileimportsImportIdWithResponse(ctx, projectID, serviceID, importID)
			if err != nil {
				return fmt.Errorf("failed to get file import: %w", err)
			}

			if resp.StatusCode() != 200 {
				return exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			// Output file import details
			return outputFileImport(cmd, resp.JSON200.FileImport, cfg.Output)
		},
	}

	cmd.Flags().VarP((*outputWithEnvFlag)(&output), "output", "o", "output format (json, yaml, table)")

	return cmd
}

// buildFileImportCancelCmd creates the cancel command
func buildFileImportCancelCmd() *cobra.Command {
	var reason string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "cancel <import-id>",
		Short: "Cancel a running file import",
		Long: `Cancel a file import that is currently running or queued.

Note: This is a destructive operation. Use --confirm to skip confirmation prompt.

Examples:
  # Cancel an import (with confirmation)
  tiger file-import cancel cli-upload-1234567890

  # Cancel without confirmation prompt
  tiger file-import cancel cli-upload-1234567890 --confirm --reason "user requested"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			importID := args[0]

			if reason == "" {
				reason = "cancelled by user via tiger-cli"
			}

			// Interactive confirmation unless --confirm
			if !confirm {
				fmt.Fprintf(cmd.ErrOrStderr(), "Are you sure you want to cancel import '%s'? (yes/no): ", importID)
				reader := bufio.NewReader(cmd.InOrStdin())
				confirmation, _ := reader.ReadString('\n')
				confirmation = strings.TrimSpace(strings.ToLower(confirmation))
				if confirmation != "yes" && confirmation != "y" {
					return fmt.Errorf("operation cancelled")
				}
			}

			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Get credentials
			apiKey, projectID, err := getCredentialsForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
			}

			serviceID := cfg.ServiceID
			if serviceID == "" {
				return fmt.Errorf("service ID is required. Set it with 'tiger config set service_id <service-id>'")
			}

			// Create API client
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Make API call to cancel
			cancelType := api.Cancel
			resp, err := client.PatchProjectsProjectIdServicesServiceIdFileimportsImportIdWithResponse(
				ctx,
				projectID,
				serviceID,
				importID,
				api.PatchProjectsProjectIdServicesServiceIdFileimportsImportIdJSONRequestBody{
					Requests: []api.UpdateFileImportRequest{
						{
							Type: cancelType,
							Cancel: &api.UpdateFileImportRequestCancel{
								Reason: reason,
							},
						},
					},
				},
			)
			if err != nil {
				return fmt.Errorf("failed to cancel file import: %w", err)
			}

			if resp.StatusCode() != 200 {
				return exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "File import cancelled successfully\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "reason for cancellation")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "skip confirmation prompt")

	return cmd
}

// Helper functions

func uploadFileToS3(presignedURL string, file io.Reader, contentLength int) error {
	req, err := http.NewRequest("PUT", presignedURL, file)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Length", fmt.Sprintf("%d", contentLength))
	req.ContentLength = int64(contentLength)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func waitForImportCompletion(ctx context.Context, client api.ClientWithResponsesInterface, projectID, serviceID, importID string, timeout time.Duration, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return exitWithCode(ExitTimeout, fmt.Errorf("timeout waiting for import to complete"))
		case <-ticker.C:
			resp, err := client.GetProjectsProjectIdServicesServiceIdFileimportsImportIdWithResponse(ctx, projectID, serviceID, importID)
			if err != nil {
				return fmt.Errorf("failed to check import status: %w", err)
			}

			if resp.StatusCode() != 200 || resp.JSON200 == nil {
				return fmt.Errorf("failed to get import status")
			}

			fileImport := resp.JSON200.FileImport
			state := string(fileImport.State.State)

			// Show progress if available
			if fileImport.State.Progress != nil {
				fmt.Fprintf(out, "Progress: %d/%d - %s\n",
					fileImport.State.Progress.Current,
					*fileImport.State.Progress.Total,
					*fileImport.State.Progress.Message,
				)
			}

			// Check terminal states
			switch state {
			case "SUCCESS":
				fmt.Fprintf(out, "Import completed successfully\n")
				return nil
			case "FAILURE":
				failureReason := "unknown error"
				if fileImport.State.FailureReason != nil {
					failureReason = *fileImport.State.FailureReason
				}
				return fmt.Errorf("import failed: %s", failureReason)
			case "CANCELLED":
				return fmt.Errorf("import was cancelled")
			}
		}
	}
}

func outputFileImportList(cmd *cobra.Command, fileImports []api.FileImport, outputFormat string) error {
	switch strings.ToLower(outputFormat) {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(fileImports)
	case "yaml":
		return util.SerializeToYAML(cmd.OutOrStdout(), fileImports, true)
	default:
		// Table output
		table := tablewriter.NewWriter(cmd.OutOrStdout())
		table.Header("ID", "STATE", "TABLE", "SIZE", "CREATED")

		for _, fi := range fileImports {
			state := string(fi.State.State)
			tableName := fi.TableIdentifier.TableName
			if fi.TableIdentifier.SchemaName != nil {
				tableName = *fi.TableIdentifier.SchemaName + "." + tableName
			}

			sizeStr := fmt.Sprintf("%d bytes", fi.Size)
			if fi.Size > 1024*1024 {
				sizeStr = fmt.Sprintf("%.2f MB", float64(fi.Size)/(1024*1024))
			} else if fi.Size > 1024 {
				sizeStr = fmt.Sprintf("%.2f KB", float64(fi.Size)/1024)
			}

			table.Append(
				fi.Id,
				state,
				tableName,
				sizeStr,
				fi.CreatedAt.Format(time.RFC3339),
			)
		}

		return table.Render()
	}
}

func outputFileImport(cmd *cobra.Command, fileImport api.FileImport, outputFormat string) error {
	switch strings.ToLower(outputFormat) {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(fileImport)
	case "yaml":
		return util.SerializeToYAML(cmd.OutOrStdout(), fileImport, true)
	default:
		// Table output
		fmt.Fprintf(cmd.OutOrStdout(), "Import ID:     %s\n", fileImport.Id)
		fmt.Fprintf(cmd.OutOrStdout(), "State:         %s\n", fileImport.State.State)

		tableName := fileImport.TableIdentifier.TableName
		if fileImport.TableIdentifier.SchemaName != nil {
			tableName = *fileImport.TableIdentifier.SchemaName + "." + tableName
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Table:         %s\n", tableName)
		fmt.Fprintf(cmd.OutOrStdout(), "Size:          %d bytes\n", fileImport.Size)
		fmt.Fprintf(cmd.OutOrStdout(), "Source Type:   %s\n", fileImport.Source.Type)
		fmt.Fprintf(cmd.OutOrStdout(), "Created:       %s\n", fileImport.CreatedAt.Format(time.RFC3339))

		if fileImport.State.FailureReason != nil && *fileImport.State.FailureReason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Failure:       %s\n", *fileImport.State.FailureReason)
		}

		if len(fileImport.Labels) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Labels:\n")
			for _, label := range fileImport.Labels {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s=%s\n", label.Key, label.Value)
			}
		}

		return nil
	}
}
