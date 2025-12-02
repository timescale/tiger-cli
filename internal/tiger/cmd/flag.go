package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// outputFlag implements the [github.com/spf13/pflag.Value] interface.
type outputFlag string

func (o *outputFlag) Set(val string) error {
	if err := config.ValidateOutputFormat(val, false); err != nil {
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
	if err := config.ValidateOutputFormat(val, true); err != nil {
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

type runE func(cmd *cobra.Command, args []string) error

var flagNameReplacer = strings.NewReplacer("-", "_")

func bindFlags(flags ...string) runE {
	return func(cmd *cobra.Command, args []string) error {
		for _, flag := range flags {
			key := flagNameReplacer.Replace(flag)
			if err := viper.BindPFlag(key, cmd.Flags().Lookup(flag)); err != nil {
				return fmt.Errorf("failed to bind %s flag: %w", flag, err)
			}
		}
		return nil
	}
}
