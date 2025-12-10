package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// connectWithPasswordMenu handles the connection flow if the stored password is invalid
// Offers an interactive menu to enter the password manually or reset it
func connectWithPasswordMenu(
	ctx context.Context,
	cmd *cobra.Command,
	client *api.ClientWithResponses,
	service api.Service,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
) error {
	// Interactive mode: Get stored password (if any)
	storage := common.GetPasswordStorage()
	storedPassword, err := storage.Get(service, details.Role)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not retrieve stored password: %v\n", err)
	}

	// Try to connect with stored password first
	err = testConnectionWithPassword(ctx, details, storedPassword)
	if err == nil {
		// Password works, launch psql
		return launchPsql(details, psqlPath, psqlFlags, service, cmd)
	}

	// Check if it's an auth error
	if !isAuthenticationError(err) {
		// Non-auth error (network, timeout, etc.) - report it directly
		return err
	}
	// Auth failed with stored password, continue to recovery menu
	fmt.Fprintf(cmd.ErrOrStderr(), "%s\nStored password is likely invalid or expired.\n\n", err.Error())

	// Check if we're in a TTY for interactive menu
	if !checkStdinIsTTY() {
		return fmt.Errorf("authentication failed and no TTY available for interactive password entry")
	}

	// Interactive recovery loop
	// Only allow password reset for admin role
	canResetPassword := details.Role == "tsdbadmin"
	for {
		option, err := selectPasswordRecoveryOption(cmd.ErrOrStderr(), canResetPassword)
		if err != nil {
			return err
		}

		switch option {
		case optionEnterPassword:
			// Prompt for password
			fmt.Fprint(cmd.ErrOrStderr(), "Enter password: ")
			password, err := readString(ctx, readPasswordFromTerminal)
			fmt.Fprintln(cmd.ErrOrStderr()) // newline after password entry
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil // user cancelled
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Error reading password: %v\n\n", err)
				continue
			}

			// Test, save, and launch
			details.Password = password
			if err = testSaveAndLaunchPsqlWithPassword(ctx, cmd, details, psqlPath, psqlFlags, service); err != nil {
				if isAuthenticationError(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "Password incorrect. Please try again.\n\n")
					continue
				}
				return fmt.Errorf("connection failed: %w", err)
			}
			return nil

		case optionResetPassword:
			// Prompt and reset
			password, err := promptAndResetPassword(ctx, cmd.ErrOrStderr(), client, service, details.Role)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil // user cancelled
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Error resetting password: %v\n\n", err)
				continue
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "✅ Master password for '%s' user updated successfully\n", details.Role)
			// Launch psql (password is now in storage)
			details.Password = password
			return launchPsql(details, psqlPath, psqlFlags, service, cmd)

		case optionExit:
			return nil
		}
	}
}

// testConnectionWithPassword tests database connectivity with a specific password
// Returns nil on success, error on failure
func testConnectionWithPassword(ctx context.Context, details *common.ConnectionDetails, password string) error {
	// copy details with provided password
	copyDetails := *details
	copyDetails.Password = password
	connStr := copyDetails.String()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	return conn.Close(ctx)
}

// passwordRecoveryOption represents the user's choice in the password recovery menu
type passwordRecoveryOption int

const (
	optionEnterPassword passwordRecoveryOption = iota
	optionResetPassword
	optionExit
)

// passwordRecoveryModel is the Bubble Tea model for password recovery selection
type passwordRecoveryModel struct {
	options          []string
	optionMap        []passwordRecoveryOption // maps cursor position to option enum
	cursor           int
	selected         passwordRecoveryOption
	canResetPassword bool
}

func newPasswordRecoveryModel(canResetPassword bool) passwordRecoveryModel {
	options := []string{"Enter password manually"}
	optionMap := []passwordRecoveryOption{optionEnterPassword}

	if canResetPassword {
		options = append(options, "Update/reset password")
		optionMap = append(optionMap, optionResetPassword)
	}

	options = append(options, "Exit")
	optionMap = append(optionMap, optionExit)

	return passwordRecoveryModel{
		options:          options,
		optionMap:        optionMap,
		cursor:           0,
		canResetPassword: canResetPassword,
	}
}

func (m passwordRecoveryModel) Init() tea.Cmd {
	return nil
}

func (m passwordRecoveryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.selected = optionExit
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.selected = m.optionMap[m.cursor]
			return m, tea.Quit
		default:
			// Handle number keys based on available options
			if len(msg.String()) == 1 && msg.String()[0] >= '1' && msg.String()[0] <= '9' {
				idx := int(msg.String()[0] - '1') // '1' -> 0, '2' -> 1, etc.
				if idx >= 0 && idx < len(m.options) {
					m.cursor = idx
					m.selected = m.optionMap[idx]
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m passwordRecoveryModel) View() string {
	s := "What would you like to do?\n\n"

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, option)
	}

	s += "\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to quit"
	return s
}

// selectPasswordRecoveryOption shows the interactive menu for password recovery
// canResetPassword controls whether the "Update/reset password" option is shown
func selectPasswordRecoveryOption(out io.Writer, canResetPassword bool) (passwordRecoveryOption, error) {
	model := newPasswordRecoveryModel(canResetPassword)

	program := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return optionExit, fmt.Errorf("failed to run password recovery menu: %w", err)
	}

	result := finalModel.(passwordRecoveryModel)
	return result.selected, nil
}

// updateAndSaveServicePassword updates a service password via API and saves it locally.
// It handles the API call and password storage.
func updateAndSaveServicePassword(
	ctx context.Context,
	client api.ClientWithResponsesInterface,
	service api.Service,
	newPassword string,
	role string,
	statusOut io.Writer,
) error {
	// Call API to update password
	updateReq := api.UpdatePasswordInput{Password: newPassword}
	resp, err := client.PostProjectsProjectIdServicesServiceIdUpdatePasswordWithResponse(ctx, *service.ProjectId, *service.ServiceId, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 204 {
		return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	// Save password locally
	if result, err := common.SavePasswordWithResult(service, newPassword, role); err != nil {
		fmt.Fprintf(statusOut, "Warning: could not save password: %v\n", err)
	} else if result.Success {
		fmt.Fprintf(statusOut, "%s\n", result.Message)
		fmt.Fprintf(statusOut, "To view your new password, run: \n\t tiger service get %s --with-password\n", util.Deref(service.ServiceId))
	}

	return nil
}

// resetServicePassword resets the password via API. If newPassword is empty, generates one.
func resetServicePassword(ctx context.Context, client api.ClientWithResponsesInterface, service api.Service, role string, newPassword string, statusOut io.Writer) (string, error) {
	// Generate password if not provided
	if newPassword == "" {
		var err error
		if newPassword, err = generateSecurePassword(32); err != nil {
			return "", fmt.Errorf("failed to generate new password: %w", err)
		}
		fmt.Fprintf(statusOut, "Successfully generated a new password.\n")
	}

	// Update and save password
	if err := updateAndSaveServicePassword(ctx, client, service, newPassword, role, statusOut); err != nil {
		return "", err
	}
	return newPassword, nil
}

// testSaveAndLaunchPsqlWithPassword tests a password, saves it if valid, and launches psql.
// Returns a retryable error if authentication fails, or a fatal error otherwise.
func testSaveAndLaunchPsqlWithPassword(
	ctx context.Context,
	cmd *cobra.Command,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
	service api.Service,
) error {
	// Test the password
	if err := testConnectionWithPassword(ctx, details, details.Password); err != nil {
		return err
	}

	// Password works! Save it
	result, saveErr := common.SavePasswordWithResult(service, details.Password, details.Role)
	if saveErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not save password: %v\n", saveErr)
	} else if result.Success {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s\n", result.Message)
	}

	// Launch psql
	return launchPsql(details, psqlPath, psqlFlags, service, cmd)
}

// promptAndResetPassword prompts for a new password and resets it via API.
// If the user leaves the password empty, a secure password is generated.
// Returns the new password on success.
func promptAndResetPassword(
	ctx context.Context,
	out io.Writer,
	client api.ClientWithResponsesInterface,
	service api.Service,
	role string,
) (string, error) {
	fmt.Fprint(out, "Enter new password (leave empty to generate): ")
	newPassword, err := readString(ctx, readPasswordFromTerminal)
	fmt.Fprintln(out) // newline after password entry
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	return resetServicePassword(ctx, client, service, role, newPassword, out)
}

// isAuthenticationError checks if the error is a PostgreSQL authentication failure
func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	// Check for PostgreSQL error code 28P01 (invalid_password) or 28000 (invalid_authorization_specification)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "28P01" || pgErr.Code == "28000"
	}
	return false
}
