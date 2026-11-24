package cmd

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildDbCreateRoleCmd(app *common.App) *cobra.Command {
	var name string
	var readOnly bool
	var from []string
	var statementTimeout time.Duration
	var password string

	cmd := &cobra.Command{
		Use:     "role [name-or-id]",
		Aliases: []string{"user"},
		Short:   "Create a new database role",
		Long: `Create a new database role with optional read-only enforcement.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. You can also pass a branch, by ID or name, to
create the role on that branch. A read replica ID is rejected, since replicas
are read-only; create the role on the primary instead.

By default, a secure random password is auto-generated for the new role. You can:
- Provide an explicit password with --password=<value>
- Use TIGER_NEW_PASSWORD environment variable
- Let it auto-generate (default)

The password is sent to the database only as a SCRAM-SHA-256 hash, so the
plaintext never reaches the server or its logs. Passwords containing characters
that database clients normalize inconsistently (such as ligatures, full-width
letters, non-ASCII spaces, or invisible characters) are refused.

The password is saved according to your --password-storage setting (keyring, pgpass, or none).

Read-Only Mode for AI Agents:
The --read-only flag enables permanent read-only enforcement at the PostgreSQL level
using the tsdb_admin.read_only_role extension setting. This is designed to provide
safe database access for AI agents and automated tools that need to read production
data without risk of modification.

Technical Details:
This command executes PostgreSQL statements in a transaction to create and configure the role.

CREATE ROLE Options Used:
  - LOGIN: Always enabled to allow the role to connect
  - PASSWORD: Always set (from flag, env var, or auto-generated), as a SCRAM-SHA-256 hash
  - IN ROLE: Added when --from flag is provided to inherit grants from existing roles

PostgreSQL Configuration Parameters That May Be Set:
  - tsdb_admin.read_only_role: Set to 'true' when --read-only flag is used
    (enforces permanent read-only mode for the role)
  - statement_timeout: Set when --statement-timeout flag is provided
    (kills queries that exceed the specified duration, in milliseconds)`,
		Example: `  # Create a role with global database access (uses default service, auto-generates password)
  tiger db create role --name ai_analyst --from tsdbadmin

  # Create a role for specific service
  tiger db create role svc-12345 --name ai_analyst

  # Create a read-only role
  tiger db create role --name ai_analyst --read-only

  # Create a read-only role with same grants as another role
  tiger db create role --name ai_analyst --read-only --from app_role

  # Create a read-only role inheriting from multiple roles
  tiger db create role --name ai_analyst --read-only --from app_role --from readonly_role

  # Create a read-only role with statement timeout
  tiger db create role --name ai_analyst --read-only --statement-timeout 30s

  # Create a role with specific password
  tiger db create role --name ai_analyst --password=my-secure-password

  # Create a role with password from environment variable
  TIGER_NEW_PASSWORD=my-secure-password tiger db create role --name ai_analyst`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate arguments
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}

			// Gated here rather than letting the server reject the CREATE ROLE.
			// Refuse the blanket case before resolving, as resolveServiceForWrite
			// does, then gate on the target's own tag.
			if cfg.ReadOnly.BlocksAll() {
				return common.ErrReadOnly
			}
			target, err := resolveConnectionTarget(cmd.Context(), client, projectID, serviceRef)
			if err != nil {
				return err
			}
			if err := common.CheckReadOnly(cfg, target.Environment); err != nil {
				return err
			}

			// A read replica is read-only, so a role can't be created there.
			if target.Kind == common.TargetReadReplica {
				return fmt.Errorf("'%s' is a read replica; create the role on its primary service '%s' instead",
					target.ID, target.Credentials.ID)
			}

			// Get password
			rolePassword, err := getPasswordForRole(password)
			if err != nil {
				return fmt.Errorf("failed to determine password: %w", err)
			}
			if err := checkPasswordNormalization(rolePassword); err != nil {
				return err
			}

			// Build connection string
			details, err := common.GetConnectionDetails(cfg, target, common.ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "tsdbadmin", // Use admin role to create new roles
				WithPassword: true,
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			// Connect to database. ConnConfig bounds the attempt to each
			// address, so the deadline below covers only creating the role.
			connConfig, err := details.ConnConfig()
			if err != nil {
				return err
			}
			conn, err := pgx.ConnectConfig(cmd.Context(), connConfig)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer conn.Close(cmd.Context())

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Create the role with all options in a transaction
			if err := createRoleWithOptions(ctx, conn, name, rolePassword, readOnly, statementTimeout, from); err != nil {
				return fmt.Errorf("failed to create role: %w", err)
			}

			// Save password to storage with the new role name
			result, err := common.SavePasswordWithResult(cfg, target.Credentials.PasswordTarget, rolePassword, name)
			if err != nil {
				cmd.PrintErrf("Warning: %s\n", result.Message)
			} else if !result.Success {
				cmd.PrintErrf("Warning: %s\n", result.Message)
			}

			// Output result in requested format
			return outputCreateRoleResult(cmd, name, readOnly, statementTimeout, from, cfg.Output)
		},
	}

	// Add flags
	cmd.Flags().StringVar(&name, "name", "", "Role name to create (required)")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Enable permanent read-only enforcement via tsdb_admin.read_only_role")
	cmd.Flags().StringSliceVar(&from, "from", []string{}, "Roles to inherit grants from (e.g., --from app_role --from readonly_role or --from app_role,readonly_role)")
	cmd.Flags().DurationVar(&statementTimeout, "statement-timeout", 0, "Set statement timeout for the role (e.g., 30s, 5m)")
	cmd.Flags().StringVar(&password, "password", "", "Password for the role. If not provided, checks TIGER_NEW_PASSWORD environment variable, otherwise auto-generates a secure random password.")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	markFlagRequired(cmd, "name")

	return cmd
}

// buildCreateRoleSQL generates the CREATE ROLE SQL statement with LOGIN, PASSWORD, and optional IN ROLE clause.
// The verifier, from scramSHA256Verifier, needs no escaping: it holds only
// base64 characters, '$', and ':'.
func buildCreateRoleSQL(roleName string, passwordVerifier string, fromRoles []string) string {
	sanitizedRoleName := pgx.Identifier{roleName}.Sanitize()
	createSQL := fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD '%s'", sanitizedRoleName, passwordVerifier)

	// Add IN ROLE clause if fromRoles is specified
	// IN ROLE adds the new role as a member of existing roles (equivalent to GRANT existing_role TO new_role)
	if len(fromRoles) > 0 {
		var sanitizedRoles []string
		for _, role := range fromRoles {
			sanitizedRoles = append(sanitizedRoles, pgx.Identifier{role}.Sanitize())
		}
		createSQL += " IN ROLE " + strings.Join(sanitizedRoles, ", ")
	}

	return createSQL
}

// buildReadOnlyAlterSQL generates the ALTER ROLE SQL statement for read-only enforcement
func buildReadOnlyAlterSQL(roleName string) string {
	sanitizedRoleName := pgx.Identifier{roleName}.Sanitize()
	return fmt.Sprintf("ALTER ROLE %s SET tsdb_admin.read_only_role = true", sanitizedRoleName)
}

// buildStatementTimeoutAlterSQL generates the ALTER ROLE SQL statement for statement timeout configuration
func buildStatementTimeoutAlterSQL(roleName string, timeout time.Duration) string {
	sanitizedRoleName := pgx.Identifier{roleName}.Sanitize()
	timeoutMs := timeout.Milliseconds()
	return fmt.Sprintf("ALTER ROLE %s SET statement_timeout = %d", sanitizedRoleName, timeoutMs)
}

// createRoleWithOptions creates a new PostgreSQL role with all specified options in a single transaction
func createRoleWithOptions(ctx context.Context, conn *pgx.Conn, roleName, rolePassword string, readOnly bool, statementTimeout time.Duration, fromRoles []string) error {
	// Begin transaction for atomic operation
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	verifier, err := scramSHA256Verifier(rolePassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Check if tsdbadmin is in the fromRoles list
	hasTsdbadmin := false
	var otherRoles []string
	for _, role := range fromRoles {
		if role == "tsdbadmin" {
			hasTsdbadmin = true
		} else {
			otherRoles = append(otherRoles, role)
		}
	}

	// If tsdbadmin is requested, use special TimescaleDB Cloud functions
	if hasTsdbadmin {
		// Enforce read-only requirement when inheriting from tsdbadmin
		if !readOnly {
			return fmt.Errorf("roles inheriting from tsdbadmin must be read-only (use --read-only flag)")
		}

		// Cannot set statement_timeout on roles created with create_bare_readonly_role
		// due to permission restrictions on altering special roles
		if statementTimeout > 0 {
			return fmt.Errorf("cannot use --statement-timeout with --from tsdbadmin (permission denied to alter special roles)")
		}

		// Use timescale_functions.create_bare_readonly_role to create the role
		// This function creates a read-only role that can inherit tsdbadmin privileges.
		// It passes the password to CREATE ROLE unchanged, so the verifier is stored as-is.
		if _, err := tx.Exec(ctx, "SELECT timescale_functions.create_bare_readonly_role($1, $2)",
			roleName, verifier); err != nil {
			return fmt.Errorf("failed to create role with create_bare_readonly_role: %w", err)
		}

		// Grant tsdbadmin privileges using the special function
		if _, err := tx.Exec(ctx, "SELECT timescale_functions.grant_tsdbadmin_to_role($1)",
			roleName); err != nil {
			return fmt.Errorf("failed to grant tsdbadmin privileges: %w", err)
		}

		// Grant any other roles (besides tsdbadmin) if specified
		// This is necessary because the special functions don't support IN ROLE clause
		for _, role := range otherRoles {
			grantSQL := fmt.Sprintf("GRANT %s TO %s",
				pgx.Identifier{role}.Sanitize(),
				pgx.Identifier{roleName}.Sanitize())
			if _, err := tx.Exec(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant role %s: %w", role, err)
			}
		}
	} else {
		// Use standard CREATE ROLE for non-tsdbadmin cases
		// IN ROLE clause handles all role grants, so no need for separate GRANT statements
		createSQL := buildCreateRoleSQL(roleName, verifier, fromRoles)
		if _, err := tx.Exec(ctx, createSQL); err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}

		// Configure read-only mode if requested
		if readOnly {
			alterSQL := buildReadOnlyAlterSQL(roleName)
			if _, err := tx.Exec(ctx, alterSQL); err != nil {
				return fmt.Errorf("failed to configure read-only mode: %w", err)
			}
		}
	}

	// Set statement timeout if requested
	if statementTimeout > 0 {
		alterSQL := buildStatementTimeoutAlterSQL(roleName, statementTimeout)
		if _, err := tx.Exec(ctx, alterSQL); err != nil {
			return fmt.Errorf("failed to set statement timeout: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// getPasswordForRole determines the password based on flags and environment
func getPasswordForRole(password string) (string, error) {
	// Priority order:
	// 1. Explicit password value from --password flag
	// 2. TIGER_NEW_PASSWORD environment variable
	// 3. Auto-generate secure random password

	if password != "" {
		// Explicit password provided via --password flag
		return password, nil
	}

	// Check environment variable
	if envPassword := os.Getenv("TIGER_NEW_PASSWORD"); envPassword != "" {
		return envPassword, nil
	}

	// Auto-generate secure password
	return common.GenerateSecurePassword(32)
}

// errPasswordNormalized is returned for a password that SASLprep would change.
var errPasswordNormalized = errors.New("password contains characters that database clients handle inconsistently, " +
	"such as ligatures, full-width letters, non-ASCII spaces, or invisible characters. Choose a different password")

// checkPasswordNormalization refuses a password that SASLprep (RFC 4013) would
// change. Postgres and libpq apply SASLprep to a password before hashing it,
// while pgx approximates it with PRECIS (RFC 8265), and the two disagree on
// some characters. A password that neither SASLprep's mapping step nor its
// NFKC normalization changes is hashed as typed by all of them, so a verifier
// built from it works with every client.
func checkPasswordNormalization(password string) error {
	if strings.ContainsFunc(password, saslPrepMaps) || !norm.NFKC.IsNormalString(password) {
		return errPasswordNormalized
	}
	return nil
}

// saslPrepMaps reports whether SASLprep's mapping step (RFC 4013, section 2)
// changes r: it removes the characters in RFC 3454 table B.1 and replaces the
// non-ASCII spaces in table C.1.2 with an ASCII space.
func saslPrepMaps(r rune) bool {
	switch {
	// Table B.1: commonly mapped to nothing.
	case r == 0x00AD, r == 0x034F, r == 0x1806, r >= 0x180B && r <= 0x180D,
		r >= 0x200B && r <= 0x200D, r == 0x2060, r >= 0xFE00 && r <= 0xFE0F, r == 0xFEFF:
		return true
	// Table C.1.2: non-ASCII space characters.
	case r == 0x00A0, r == 0x1680, r >= 0x2000 && r <= 0x200B, r == 0x202F, r == 0x205F, r == 0x3000:
		return true
	}
	return false
}

// SCRAM-SHA-256 parameters, matching what libpq and Postgres use by default.
const (
	scramSaltLen    = 16
	scramIterations = 4096
)

// scramSHA256Verifier hashes password into the SCRAM-SHA-256 verifier
// Postgres stores for it, as libpq's PQencryptPasswordConn does. Postgres
// stores a password already in this form as-is, so sending the verifier keeps
// the plaintext off the server and out of its logs. The password is hashed as
// typed, so it must pass checkPasswordNormalization first.
func scramSHA256Verifier(password string) (string, error) {
	salt := make([]byte, scramSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	return buildSCRAMSHA256Verifier(password, salt, scramIterations)
}

// buildSCRAMSHA256Verifier builds the verifier in the form Postgres stores it,
// SCRAM-SHA-256$<iterations>:<salt>$<StoredKey>:<ServerKey> (RFC 5802, RFC
// 7677).
func buildSCRAMSHA256Verifier(password string, salt []byte, iterations int) (string, error) {
	saltedPassword, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return "", err
	}
	clientKey, err := hmacSHA256(saltedPassword, "Client Key")
	if err != nil {
		return "", err
	}
	storedKey := sha256.Sum256(clientKey)
	serverKey, err := hmacSHA256(saltedPassword, "Server Key")
	if err != nil {
		return "", err
	}

	enc := base64.StdEncoding
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s",
		iterations,
		enc.EncodeToString(salt),
		enc.EncodeToString(storedKey[:]),
		enc.EncodeToString(serverKey),
	), nil
}

func hmacSHA256(key []byte, message string) ([]byte, error) {
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write([]byte(message)); err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

// CreateRoleResult represents the output of a create role operation
type CreateRoleResult struct {
	RoleName         string   `json:"role_name"`
	ReadOnly         bool     `json:"read_only,omitempty"`
	StatementTimeout string   `json:"statement_timeout,omitempty"`
	FromRoles        []string `json:"from_roles,omitempty"`
}

// outputCreateRoleResult formats and outputs the create role result
func outputCreateRoleResult(cmd *cobra.Command, roleName string, readOnly bool, statementTimeout time.Duration, fromRoles []string, format string) error {
	result := CreateRoleResult{
		RoleName: roleName,
		ReadOnly: readOnly,
	}

	if statementTimeout > 0 {
		result.StatementTimeout = statementTimeout.String()
	}

	if len(fromRoles) > 0 {
		result.FromRoles = fromRoles
	}

	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, result)
	case "yaml":
		return util.SerializeToYAML(outputWriter, result)
	default: // table format
		cmd.Printf("Role '%s' created\n", roleName)
		if readOnly {
			cmd.Printf("Read-only: enabled\n")
		}
		if statementTimeout > 0 {
			cmd.Printf("Statement timeout: %s\n", statementTimeout)
		}
		if len(fromRoles) > 0 {
			cmd.Printf("Inherits from: %s\n", strings.Join(fromRoles, ", "))
		}
		return nil
	}
}
