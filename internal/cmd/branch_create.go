package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildBranchCreateCmd creates the create subcommand of `tiger branch`, the
// same command as `tiger service branch` (see newBranchCreateCmd).
func buildBranchCreateCmd(app *common.App) *cobra.Command {
	return newBranchCreateCmd(app, "create [service-name-or-id]", "tiger branch create")
}

// newBranchCreateCmd builds the command that creates a branch of a service.
// `tiger branch create` and `tiger service branch` are the same command in two
// places in the tree, so both are built here, differing only in their use line
// and the command path their examples show.
func newBranchCreateCmd(app *common.App, use, commandPath string) *cobra.Command {
	var name string
	var cpu string
	var memory string
	var iops int
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   use,
		Short: "Create a branch of a database service",
		Long: `Create a branch of a Fluid Service.

A branch is a lightweight, ephemeral copy of a service, sharing the service's
storage as of the moment it is created and getting its own compute. The
tsdbadmin user gets a new password, which is saved the same way as a new
service's.

By default:
- The name is auto-generated
- The branch gets 0.25 CPU and 2GB of memory, whatever the service's size

The service can be given by ID or name as an argument, or will use the default
service from your configuration. Branching from a branch isn't supported.

` + branchCPUMemoryHelp + `

The allowed --iops values depend on your plan and the branch's CPU/memory
allocation. See the Tiger Data documentation for details. If --iops is not
given, the branch gets the baseline IOPS for your plan and its CPU allocation.`,
		Example: fmt.Sprintf(`  # Branch a service
  %[1]s svc-12345

  # Branch a service with a custom name
  %[1]s svc-12345 --name ci-4821

  # Branch a service with custom compute
  %[1]s svc-12345 --cpu 2000 --memory 16

  # Branch a service with provisioned IOPS
  %[1]s svc-12345 --cpu 2000 --iops 16000

  # Branch for a script
  %[1]s svc-12345 --name ci-4821 -o json`, commandPath),
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateWaitTimeout(waitTimeout); err != nil {
				return err
			}

			cpuMemoryCfg, err := common.FluidCPUMemoryConfigs().ValidateAndNormalize(cpu, memory)
			if err != nil {
				return err
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Gate on the branch's own tag, which is always DEV: under prod
			// mode, branching a PROD service is allowed, as forking one into
			// a DEV fork is.
			if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
				return err
			}

			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}
			parent, err := resolveService(cmd.Context(), client, projectID, serviceRef)
			if err != nil {
				return err
			}

			if name != "" {
				cmd.PrintErrf("Creating branch '%s' from service %s...\n", name, serviceLabel(*parent))
			} else {
				cmd.PrintErrf("Creating branch from service %s...\n", serviceLabel(*parent))
			}

			var createReq api.BranchCreate
			if name != "" {
				createReq.Name = &name
			}
			if cpuMemoryCfg != nil {
				createReq.CPUMillis = &cpuMemoryCfg.CPUMillis
				createReq.MemoryGbs = &cpuMemoryCfg.MemoryGBs
			}
			if cmd.Flags().Changed("iops") {
				createReq.Iops = &iops
			}

			resp, err := client.CreateBranchWithResponse(cmd.Context(), projectID, parent.ServiceID, createReq)
			if err != nil {
				return fmt.Errorf("failed to create branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return errors.New("empty response from API")
			}
			branch := *resp.JSON202

			cmd.PrintErrf("Branch ID: %s\n", branch.ID)

			// Saved before any waiting, so the password is stored even if the
			// wait fails or is interrupted.
			passwordSaved := handlePasswordSaving(cmd, cfg, common.BranchPasswordTarget(branch), util.Deref(branch.InitialPassword))

			var waitErr error
			if noWait {
				cmd.PrintErrf("Branch is being created. Use 'tiger branch get %s' to check branch status.\n", branch.ID)
			} else {
				cmd.PrintErrf("Waiting for branch to be ready (timeout: %v)...\n", waitTimeout)
				if waitErr = common.WaitForBranch(cmd.Context(), common.WaitForBranchArgs{
					Client:       client,
					ProjectID:    projectID,
					ServiceID:    parent.ServiceID,
					BranchID:     branch.ID,
					Branch:       &branch,
					TargetStatus: api.DeployStatusREADY,
					Input:        cmd.InOrStdin(),
					Output:       cmd.ErrOrStderr(),
					Timeout:      waitTimeout,
					TimeoutMsg:   "branch may still be provisioning",
				}); waitErr != nil {
					cmd.PrintErrf("Error: %s\n", waitErr)
				} else {
					cmd.PrintErrln("Branch is ready.")
					// A branch is never the default, so the command names it.
					printConnectMessage(cmd, passwordSaved, true, branch.ID)
				}
			}

			if err := outputBranch(cmd, cfg, branch, branchOutputOptions{ServiceName: parent.Name}); err != nil {
				cmd.PrintErrf("Warning: failed to output branch details: %v\n", err)
			}

			// Return error for sake of exit code, but silence it since it was already output above
			cmd.SilenceErrors = true
			return waitErr
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name for the branch (auto-generated if not provided)")
	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in millicores (default 250)")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes (default 2)")
	cmd.Flags().IntVar(&iops, "iops", 0, "Provisioned IOPS")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for the operation to complete (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")

	registerFlagCompletion(cmd, "output", outputCompletion("env"))
	registerFlagCompletion(cmd, "cpu", branchCPUCompletion)
	registerFlagCompletion(cmd, "memory", branchMemoryCompletion)
	registerFlagCompletion(cmd, "iops", iopsCompletion)

	return cmd
}
