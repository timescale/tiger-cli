package cmd

import "github.com/timescale/tiger-cli/internal/tiger/config"

// outputFlag implements the [github.com/spf13/pflag.Value] interface.
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
