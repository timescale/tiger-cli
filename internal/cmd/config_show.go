package cmd

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildConfigShowCmd(app *common.App) *cobra.Command {
	var noDefaults bool
	var withEnv bool

	cmd := &cobra.Command{
		Use:               "show",
		Aliases:           []string{"list", "ls"},
		Short:             "Show current configuration",
		Long:              `Display the current CLI configuration settings`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			// Values are re-read free of env and CLI flags (unless --with-env
			// is given), so `config show -o json` reports the configured
			// `output` value rather than the flag's.
			cfgOut, err := config.LoadForOutput(cfg.ConfigDir, withEnv, noDefaults)
			if err != nil {
				return err
			}

			output := cmd.OutOrStdout()
			switch cfg.Output {
			case "json":
				return util.SerializeToJSON(output, cfgOut)
			case "yaml":
				return util.SerializeToYAML(output, cfgOut)
			default:
				return outputTable(output, cfgOut)
			}
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "output format (json, yaml, table)")
	cmd.Flags().BoolVar(&noDefaults, "no-defaults", false, "do not show default values for unset fields")
	cmd.Flags().BoolVar(&withEnv, "with-env", false, "apply environment variable overrides")

	return cmd
}

// outputTable renders the config as a table, rows sorted by property name
// (matching the JSON field order and YAML's sorted keys).
func outputTable(w io.Writer, cfg *config.ConfigOutput) error {
	table := tablewriter.NewWriter(w)
	table.Header("PROPERTY", "VALUE")
	if cfg.Analytics != nil {
		table.Append("analytics", fmt.Sprintf("%t", *cfg.Analytics))
	}
	if cfg.APIURL != nil {
		table.Append("api_url", *cfg.APIURL)
	}
	if cfg.Color != nil {
		table.Append("color", fmt.Sprintf("%t", *cfg.Color))
	}
	if cfg.ConsoleURL != nil {
		table.Append("console_url", *cfg.ConsoleURL)
	}
	if cfg.DocsMCP != nil {
		table.Append("docs_mcp", fmt.Sprintf("%t", *cfg.DocsMCP))
	}
	if cfg.DocsMCPURL != nil {
		table.Append("docs_mcp_url", *cfg.DocsMCPURL)
	}
	if cfg.GatewayURL != nil {
		table.Append("gateway_url", *cfg.GatewayURL)
	}
	if cfg.MCPMaxRows != nil {
		table.Append("mcp_max_rows", fmt.Sprintf("%d", *cfg.MCPMaxRows))
	}
	if cfg.Output != nil {
		table.Append("output", *cfg.Output)
	}
	if cfg.PasswordStorage != nil {
		table.Append("password_storage", *cfg.PasswordStorage)
	}
	if cfg.ReadOnly != nil {
		table.Append("read_only", fmt.Sprintf("%t", *cfg.ReadOnly))
	}
	if cfg.ReleasesURL != nil {
		table.Append("releases_url", *cfg.ReleasesURL)
	}
	if cfg.ServiceID != nil {
		table.Append("service_id", *cfg.ServiceID)
	}
	if cfg.VersionCheck != nil {
		table.Append("version_check", fmt.Sprintf("%t", *cfg.VersionCheck))
	}
	return table.Render()
}
