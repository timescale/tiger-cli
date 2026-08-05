package cmd

import (
	"github.com/timescale/tiger-cli/internal/config"
)

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
