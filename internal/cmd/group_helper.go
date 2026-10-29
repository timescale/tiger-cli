package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// groupRun shows a group command's help when it's run without a subcommand.
// It's a Run rather than a RunE so it skips the per-invocation lifecycle: a
// group command never needs the config or client.
func groupRun(cmd *cobra.Command, _ []string) {
	cmd.HelpFunc()(cmd, nil)
}

// groupArgs reports an argument to a group command as an unknown subcommand,
// with the suggestions cobra only offers for the root command itself.
func groupArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	// cobra's own default, which only its root-command path applies.
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(suggestions, "\n\t")
	}
	return errors.New(msg)
}
