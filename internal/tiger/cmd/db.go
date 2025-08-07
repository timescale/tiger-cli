package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

var (
	// getAPIKeyForDB can be overridden for testing
	getAPIKeyForDB = getAPIKey
)

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations and management",
	Long:  `Database-specific operations including connection management, testing, and configuration.`,
}

// dbConnectionStringCmd represents the connection-string command under db
var dbConnectionStringCmd = &cobra.Command{
	Use:   "connection-string [service-id]",
	Short: "Get connection string for a service",
	Long: `Get a PostgreSQL connection string for connecting to a database service.

The service ID can be provided as an argument or will use the default service
from your configuration. The connection string includes all necessary parameters
for establishing a database connection to the TimescaleDB/PostgreSQL service.

Examples:
  # Get connection string for default service
  tiger db connection-string

  # Get connection string for specific service
  tiger db connection-string svc-12345

  # Get pooled connection string (uses connection pooler if available)
  tiger db connection-string svc-12345 --pooled

  # Get connection string with custom role/username
  tiger db connection-string svc-12345 --role readonly`,
	RunE: func(cmd *cobra.Command, args []string) error {
		service, err := getServiceDetails(cmd, args)
		if err != nil {
			return err
		}

		connectionString, err := buildConnectionString(service, dbConnectionStringPooled, dbConnectionStringRole, cmd)
		if err != nil {
			return fmt.Errorf("failed to build connection string: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), connectionString)
		return nil
	},
}

// dbConnectCmd represents the connect/psql command under db
var dbConnectCmd = &cobra.Command{
	Use:     "connect [service-id]",
	Aliases: []string{"psql"},
	Short:   "Connect to a database",
	Long: `Connect to a database service using psql client.

The service ID can be provided as an argument or will use the default service
from your configuration. This command will launch an interactive psql session
with the appropriate connection parameters.

Authentication is handled automatically using:
1. ~/.pgpass file (if password was saved during service creation)  
2. PGPASSWORD environment variable
3. Interactive password prompt (if neither above is available)

Examples:
  # Connect to default service
  tiger db connect
  tiger db psql

  # Connect to specific service
  tiger db connect svc-12345
  tiger db psql svc-12345

  # Connect using connection pooler (if available)
  tiger db connect svc-12345 --pooled
  tiger db psql svc-12345 --pooled

  # Connect with custom role/username
  tiger db connect svc-12345 --role readonly
  tiger db psql svc-12345 --role readonly

  # Pass additional flags to psql (use -- to separate)
  tiger db connect svc-12345 -- --single-transaction --quiet
  tiger db psql svc-12345 -- -c "SELECT version();" --no-psqlrc`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Separate service ID from additional psql flags
		serviceArgs, psqlFlags := separateServiceAndPsqlArgs(cmd, args)

		service, err := getServiceDetails(cmd, serviceArgs)
		if err != nil {
			return err
		}

		// Check if psql is available
		psqlPath, err := exec.LookPath("psql")
		if err != nil {
			return fmt.Errorf("psql client not found. Please install PostgreSQL client tools")
		}

		// Get connection string using existing logic
		connectionString, err := buildConnectionString(service, dbConnectPooled, dbConnectRole, cmd)
		if err != nil {
			return fmt.Errorf("failed to build connection string: %w", err)
		}

		// Launch psql with additional flags
		return launchPsqlWithConnectionString(connectionString, psqlPath, psqlFlags, cmd)
	},
}

// Command-line flags for db commands
var (
	dbConnectionStringPooled bool
	dbConnectionStringRole   string
	dbConnectPooled          bool
	dbConnectRole            string
)

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbConnectionStringCmd)
	dbCmd.AddCommand(dbConnectCmd)

	// Add flags for db connection-string command
	dbConnectionStringCmd.Flags().BoolVar(&dbConnectionStringPooled, "pooled", false, "Use connection pooling")
	dbConnectionStringCmd.Flags().StringVar(&dbConnectionStringRole, "role", "tsdbadmin", "Database role/username")

	// Add flags for db connect command (works for both connect and psql)
	dbConnectCmd.Flags().BoolVar(&dbConnectPooled, "pooled", false, "Use connection pooling")
	dbConnectCmd.Flags().StringVar(&dbConnectRole, "role", "tsdbadmin", "Database role/username")
}

// buildConnectionString creates a PostgreSQL connection string from service details
func buildConnectionString(service api.Service, pooled bool, role string, cmd *cobra.Command) (string, error) {
	if service.Endpoint == nil {
		return "", fmt.Errorf("service endpoint not available")
	}

	var endpoint *api.Endpoint
	var host string
	var port int

	// Use pooler endpoint if requested and available, otherwise use direct endpoint
	if pooled && service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil {
		endpoint = service.ConnectionPooler.Endpoint
	} else {
		// If pooled was requested but no pooler is available, warn the user
		if pooled {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: Connection pooler not available for this service, using direct connection\n")
		}
		endpoint = service.Endpoint
	}

	if endpoint.Host == nil {
		return "", fmt.Errorf("endpoint host not available")
	}
	host = *endpoint.Host

	if endpoint.Port != nil {
		port = *endpoint.Port
	} else {
		port = 5432 // Default PostgreSQL port
	}

	// Database is always "tsdb" for TimescaleDB/PostgreSQL services
	database := "tsdb"

	// Build connection string in PostgreSQL URI format
	connectionString := fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", role, host, port, database)

	return connectionString, nil
}

// getServiceDetails is a helper that handles common service lookup logic and returns the service details
func getServiceDetails(cmd *cobra.Command, args []string) (api.Service, error) {
	// Get config
	cfg, err := config.Load()
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to load config: %w", err)
	}

	projectID := cfg.ProjectID
	if projectID == "" {
		return api.Service{}, fmt.Errorf("project ID is required. Set it using login with --project-id")
	}

	// Determine service ID
	var serviceID string
	if len(args) > 0 {
		serviceID = args[0]
	} else {
		serviceID = cfg.ServiceID
	}

	if serviceID == "" {
		return api.Service{}, fmt.Errorf("service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")
	}

	cmd.SilenceUsage = true

	// Get API key for authentication
	apiKey, err := getAPIKeyForDB()
	if err != nil {
		return api.Service{}, fmt.Errorf("authentication required: %w", err)
	}

	// Create API client
	client, err := api.NewTigerClient(apiKey)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to create API client: %w", err)
	}

	// Fetch service details
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to fetch service details: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return api.Service{}, fmt.Errorf("empty response from API")
		}

		return *resp.JSON200, nil

	case 401, 403:
		return api.Service{}, fmt.Errorf("authentication failed: invalid API key")
	case 404:
		return api.Service{}, fmt.Errorf("service '%s' not found in project '%s'", serviceID, projectID)
	default:
		return api.Service{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// ArgsLenAtDashProvider defines the interface for getting ArgsLenAtDash
type ArgsLenAtDashProvider interface {
	ArgsLenAtDash() int
}

// separateServiceAndPsqlArgs separates service arguments from psql flags using Cobra's ArgsLenAtDash
func separateServiceAndPsqlArgs(cmd ArgsLenAtDashProvider, args []string) ([]string, []string) {
	serviceArgs := []string{}
	psqlFlags := []string{}

	argsLenAtDash := cmd.ArgsLenAtDash()
	if argsLenAtDash >= 0 {
		// There was a -- separator
		serviceArgs = args[:argsLenAtDash]
		psqlFlags = args[argsLenAtDash:]
	} else {
		// No -- separator
		serviceArgs = args
	}

	return serviceArgs, psqlFlags
}

// launchPsqlWithConnectionString launches psql using the connection string and additional flags
func launchPsqlWithConnectionString(connectionString, psqlPath string, additionalFlags []string, cmd *cobra.Command) error {
	// Build command arguments: connection string first, then additional flags
	args := []string{connectionString}
	args = append(args, additionalFlags...)

	psqlCmd := exec.Command(psqlPath, args...)
	psqlCmd.Stdin = os.Stdin
	psqlCmd.Stdout = os.Stdout
	psqlCmd.Stderr = os.Stderr

	return psqlCmd.Run()
}
