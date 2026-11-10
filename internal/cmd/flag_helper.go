package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/timescale/tiger-cli/internal/config"
)

// markFlagRequired marks a flag as required. A failure means the flag doesn't
// exist — a programming error in the command tree that would otherwise go
// unnoticed until someone ran the command without the flag and it was accepted.
func markFlagRequired(cmd *cobra.Command, flag string) {
	if err := cmd.MarkFlagRequired(flag); err != nil {
		panic(fmt.Sprintf("%s: %v", cmd.CommandPath(), err))
	}
}

// markFlagHidden hides a flag from help output. It looks the flag up through
// cmd.Flag so it works for persistent flags too, which cmd.Flags() doesn't
// contain until parsing merges them in. A missing flag is a programming error in
// the command tree that would otherwise go unnoticed until the flag showed up in
// help output.
func markFlagHidden(cmd *cobra.Command, flag string) {
	f := cmd.Flag(flag)
	if f == nil {
		panic(fmt.Sprintf("%s: no such flag -%s", cmd.CommandPath(), flag))
	}
	f.Hidden = true
}

// registerFlagCompletion registers a shell completion function for a flag. A
// failure means the flag doesn't exist or already has a completion — either is
// a programming error in the command tree that would otherwise go unnoticed
// until someone pressed Tab.
func registerFlagCompletion(cmd *cobra.Command, flag string, f cobra.CompletionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(flag, f); err != nil {
		panic(fmt.Sprintf("%s: %v", cmd.CommandPath(), err))
	}
}

// hideBoolFlagSyntax keeps bool flags rendering as `--flag` in help output and
// the generated docs, rather than the `--flag[=true|false]` pflag prints since
// v1.1.0, which has no option to turn it off
// (https://github.com/spf13/pflag/issues/454). It wraps the value of every bool
// flag in the tree rooted at cmd in a boolFlagValue. The help flag is added
// first, since cobra otherwise adds it only once the command runs.
//
// It visits each command's LocalFlags — the flags declared on that command,
// persistent or not — so every flag is wrapped exactly once, by the command
// that owns it, however many subcommands inherit it.
func hideBoolFlagSyntax(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Value.Type() == "bool" {
			f.Value = boolFlagValue{f.Value}
		}
	})
	for _, child := range cmd.Commands() {
		hideBoolFlagSyntax(child)
	}
}

// boolFlagValue hides a bool flag value's IsBoolFlag method, which is what
// makes pflag print `[=true|false]`: embedding pflag.Value promotes only String,
// Set, and Type. Parsing is unaffected, because pflag already set the flag's
// NoOptDefVal to "true" when it was added, and that's what lets a bare `--flag`
// mean true.
type boolFlagValue struct {
	pflag.Value
}

// outputFlag implements the [github.com/spf13/pflag.Value] interface. These
// types only validate the value at parse time — commands read the result from
// cfg.Output — so they're registered with `new(outputFlag)` and no variable.
type outputFlag string

func (o *outputFlag) Set(val string) error {
	if err := config.ValidateOutputFormat(val); err != nil {
		return err
	}
	*o = outputFlag(val)
	return nil
}

func (o *outputFlag) String() string {
	return string(*o)
}

func (o *outputFlag) Type() string {
	return "string"
}

// outputWithEnvFlag implements the [github.com/spf13/pflag.Value] interface.
type outputWithEnvFlag string

func (o *outputWithEnvFlag) Set(val string) error {
	if err := config.ValidateOutputFormat(val, "env"); err != nil {
		return err
	}
	*o = outputWithEnvFlag(val)
	return nil
}

func (o *outputWithEnvFlag) String() string {
	return string(*o)
}

func (o *outputWithEnvFlag) Type() string {
	return "string"
}

// outputWithBareFlag implements the [github.com/spf13/pflag.Value] interface.
type outputWithBareFlag string

func (o *outputWithBareFlag) Set(val string) error {
	if err := config.ValidateOutputFormat(val, "bare"); err != nil {
		return err
	}
	*o = outputWithBareFlag(val)
	return nil
}

func (o *outputWithBareFlag) String() string {
	return string(*o)
}

func (o *outputWithBareFlag) Type() string {
	return "string"
}
