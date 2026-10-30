package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// branchCPUMemoryHelp is the table of CPU/Memory configurations a branch can
// be created or resized onto.
const branchCPUMemoryHelp = "Allowed CPU/Memory Configurations:\n" + fluidCPUMemoryHelp

// buildBranchCmd creates the branch command with all subcommands. Branches are
// backed by gateway endpoints marked `x-tigerdata-preview: true`, so the whole
// group is only added to the tree behind the experimental gate (see root.go).
func buildBranchCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "branch",
		Aliases: []string{"branches"},
		Short:   "Manage branches of database services",
		Long: `Manage branches of Fluid Services.

A branch is a lightweight, ephemeral copy of a Fluid Service, for development,
testing, CI/CD, and agent workflows. It shares its parent's storage as of the
moment it was created and gets its own compute.`,
	}

	cmd.AddCommand(buildBranchCreateCmd(app))
	cmd.AddCommand(buildBranchListCmd(app))
	cmd.AddCommand(buildBranchGetCmd(app))
	cmd.AddCommand(buildBranchDeleteCmd(app))
	cmd.AddCommand(buildBranchStartCmd(app))
	cmd.AddCommand(buildBranchStopCmd(app))
	cmd.AddCommand(buildBranchResizeCmd(app))
	cmd.AddCommand(buildBranchRenameCmd(app))
	cmd.AddCommand(buildBranchUpdatePasswordCmd(app))

	return cmd
}

// OutputBranch represents a branch with computed fields for output.
type OutputBranch struct {
	api.Branch
	// ServiceName is the parent service's name, when it's known.
	ServiceName string `json:"service_name,omitempty"`
	common.ConnectionDetails
	ConnectionString string `json:"connection_string,omitempty"`
	ConsoleURL       string `json:"console_url"`
}

// branchOutputOptions controls what the branch output functions include.
// The zero value leaves out every optional field.
type branchOutputOptions struct {
	// ServiceName is the parent service's name, if it's known. A branch
	// carries only its parent's ID.
	ServiceName string
	// WithPassword includes the tsdbadmin password, if one is available.
	WithPassword bool
	// RequirePassword fails the output when WithPassword finds no password.
	RequirePassword bool
}

// outputBranch outputs a single branch in the configured output format.
func outputBranch(cmd *cobra.Command, cfg *config.Config, branch api.Branch, opts branchOutputOptions) error {
	outputBr := prepareBranchForOutput(cmd, cfg, branch, opts)
	if opts.RequirePassword && opts.WithPassword && outputBr.Password == "" {
		return fmt.Errorf("password requested but not available for branch %s", outputBr.ID)
	}
	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(cfg.Output) {
	case "json":
		return util.SerializeToJSON(outputWriter, outputBr)
	case "yaml":
		return util.SerializeToYAML(outputWriter, outputBr)
	case "env":
		return outputBranchEnv(cmd, outputBr)
	default: // table format (default)
		return outputBranchTable(outputWriter, outputBr)
	}
}

// outputBranchEnv outputs branch connection details in environment variable
// format. PGPASSWORD is set only when the password was requested and found.
func outputBranchEnv(cmd *cobra.Command, branch OutputBranch) error {
	cmd.Printf("PGHOST=%s\n", branch.Host)
	cmd.Printf("PGPORT=%d\n", branch.Port)
	cmd.Printf("PGDATABASE=%s\n", branch.Database)
	cmd.Printf("PGUSER=%s\n", branch.Role)
	if branch.Password != "" {
		cmd.Printf("PGPASSWORD=%s\n", branch.Password)
	}
	return nil
}

// outputBranchTable outputs detailed branch information in a formatted table.
func outputBranchTable(output io.Writer, branch OutputBranch) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROPERTY", "VALUE")

	table.Append("Branch ID", branch.ID)
	table.Append("Name", branch.Name)
	if branch.ServiceName != "" {
		table.Append("Parent Service", fmt.Sprintf("%s (%s)", branch.ServiceName, branch.ServiceID))
	} else {
		table.Append("Parent Service", branch.ServiceID)
	}
	table.Append("Status", string(branch.Status))
	table.Append("CPU", fmt.Sprintf("%s cores (%dm)", common.FormatCPUCores(branch.CPUMillis), branch.CPUMillis))
	table.Append("Memory", fmt.Sprintf("%d GB", branch.MemoryGbs))
	if branch.Iops != nil {
		table.Append("IOPS", fmt.Sprintf("%d", *branch.Iops))
	}
	if branch.Endpoint != nil && branch.Endpoint.Host != nil && branch.Endpoint.Port != nil {
		table.Append("Direct Endpoint", fmt.Sprintf("%s:%d", *branch.Endpoint.Host, *branch.Endpoint.Port))
	}
	table.Append("Created", branch.Created.Format("2006-01-02 15:04:05 MST"))
	if branch.Password != "" {
		table.Append("Password", branch.Password)
	}
	if branch.ConnectionString != "" {
		table.Append("Connection String", branch.ConnectionString)
	}
	table.Append("Console URL", branch.ConsoleURL)

	return table.Render()
}

// prepareBranchForOutput builds the output view of a branch. cmd may be nil,
// in which case the connection-details warning is dropped rather than printed.
func prepareBranchForOutput(cmd *cobra.Command, cfg *config.Config, branch api.Branch, opts branchOutputOptions) OutputBranch {
	outputBr := OutputBranch{
		Branch:      branch,
		ServiceName: opts.ServiceName,
	}
	outputBr.Branch.InitialPassword = nil

	detailsOpts := common.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    opts.WithPassword,
		InitialPassword: util.Deref(branch.InitialPassword),
		ReadOnly:        common.CheckReadOnly(cfg, common.BranchEnvironmentTag) != nil,
	}
	if details, err := common.GetConnectionDetails(cfg, common.NewBranchConnectionTarget(branch), detailsOpts); err != nil {
		if cmd != nil {
			cmd.PrintErrf("Warning: failed to get connection details: %v\n", err)
		}
	} else {
		outputBr.ConnectionDetails = *details
		outputBr.ConnectionString = details.String()
	}

	outputBr.ConsoleURL = fmt.Sprintf("%s/projects/%s/services/%s/branches/%s/overview", cfg.ConsoleURL, branch.ProjectID, branch.ServiceID, branch.ID)

	return outputBr
}
