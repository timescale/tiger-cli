package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildFeedbackCmd creates the feedback command, which relays a message to the
// Tiger Data team.
func buildFeedbackCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:   "feedback [message]",
		Short: "Submit feedback, a bug report, or a support request",
		Long: `Submit feedback, a bug report, or a support request to the Tiger Data team.

The message is sent with the email address of your account, so the team can
follow up, and with the CLI version and operating system. Pass the message as
an argument, or omit it to read from stdin.

This does not open a support case and returns no ticket to track. For anything
that needs a tracked response, contact support directly.`,
		Example: `  # Submit feedback as an argument
  tiger feedback "I can't connect to my service after resuming it"

  # Submit feedback from stdin
  echo "Great tool!" | tiger feedback

  # Submit feedback interactively
  tiger feedback
  # → Enter your feedback (press Ctrl+D when done):`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		// The message is free text that may quote connection strings, queries,
		// or anything else, so wrapCommands keeps it out of analytics.
		Annotations: map[string]string{annotationRedactArgs: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := app.GetClient()
			if err != nil {
				return err
			}

			var message string
			if len(args) > 0 {
				message = args[0]
			} else {
				// Reading piped input, so this is deliberately not gated on a
				// TTY — only the hint is, for someone who ran the command with
				// no message.
				if util.IsTerminal(cmd.InOrStdin()) {
					cmd.PrintErrln("Enter your feedback (press Ctrl+D when done):")
				}
				message, err = util.ReadAll(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read feedback: %w", err)
				}
			}
			// ReadAll already trims piped input; trim the argument too so both
			// paths send the same message for the same text.
			message = strings.TrimSpace(message)
			if message == "" {
				return errors.New("feedback message cannot be empty")
			}

			resp, err := client.SubmitFeedbackWithResponse(cmd.Context(), api.SubmitFeedbackJSONRequestBody{
				Message: message,
				Source:  new(api.SubmitFeedbackJSONBodySourceCLI),
			})
			if err != nil {
				return fmt.Errorf("failed to submit feedback: %w", err)
			}
			if resp.StatusCode() != http.StatusNoContent {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			cmd.Println("Feedback submitted! Thank you.")
			return nil
		},
	}
}
