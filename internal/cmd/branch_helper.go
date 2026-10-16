package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// branchCPUMemoryHelp is the table of CPU/Memory configurations a branch can
// be created or resized onto.
const branchCPUMemoryHelp = "Allowed CPU/Memory Configurations:\n" + fluidCPUMemoryHelp

// newBranchCreateCmd builds the command that creates a branch of a service.
// `tiger branch create` and `tiger service branch` are the same command in two
// places in the tree, so both are built here, differing only in their use line
// and the command path their examples show.
func newBranchCreateCmd(app *common.App, use, commandPath string) *cobra.Command {
	var name string
	var cpu string
	var memory string
	var noSetDefault bool
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   use,
		Short: "Create a branch of a database service",
		Long: `Create a branch of a Fluid Service.

A branch is a lightweight, ephemeral copy of a service, sharing the service's
storage as of the moment it is created and getting its own compute. It has the
same users and passwords as the service it was created from, and its password
is copied from the service's saved password.

By default:
- The name is auto-generated
- The branch gets 0.25 CPU and 2GB of memory, whatever the service's size
- The new branch is set as the default, in place of the default service

The service can be given by ID or name as an argument, or will use the default
service from your configuration. Branching from a branch isn't supported.

` + branchCPUMemoryHelp,
		Example: fmt.Sprintf(`  # Branch a service
  %[1]s svc-12345

  # Branch a service with a custom name
  %[1]s svc-12345 --name ci-4821

  # Branch a service with custom compute
  %[1]s svc-12345 --cpu 2000 --memory 16

  # Branch for a script, without changing the default
  %[1]s svc-12345 --name ci-4821 --no-set-default -o json`, commandPath),
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			cpuMemoryCfg, err := common.BranchCPUMemoryConfigs().ValidateAndNormalize(cpu, memory)
			if err != nil {
				return err
			}

			// TODO: when the default is a branch, this fails as a service that
			// doesn't exist. Once refs can be told apart, fail with "branching
			// from a branch isn't supported yet; pass a service ID" instead.
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

			createReq := api.BranchCreate{ServiceID: parent.ServiceID}
			if name != "" {
				createReq.Name = &name
			}
			if cpuMemoryCfg != nil {
				createReq.CPUMillis = &cpuMemoryCfg.CPUMillis
				createReq.MemoryGbs = &cpuMemoryCfg.MemoryGBs
			}

			resp, err := client.CreateBranchWithResponse(cmd.Context(), projectID, createReq)
			if err != nil {
				return fmt.Errorf("failed to create branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			branch := *resp.JSON202

			cmd.PrintErrf("Branch ID: %s\n", branch.ID)
			cmd.PrintErrf("Users and passwords are the same as service %s.\n", serviceLabel(*parent))

			// Copied before any waiting, so the password is stored even if the
			// wait fails or is interrupted.
			// TODO: with pgpass storage this needs the branch's endpoint, which
			// the create response may not carry yet.
			result, _ := common.CopyParentPassword(cfg, *parent, branch, "tsdbadmin")
			printPasswordStorageResult(cmd, result)

			if !noSetDefault {
				if err := setDefaultBranch(cmd, cfg, branch.ID); err != nil {
					cmd.PrintErrf("Warning: failed to set branch as default: %v\n", err)
				}
			}

			// TODO: print "Connect with: tiger db psql" once the db commands
			// can connect to a branch.
			var waitErr error
			if noWait {
				cmd.PrintErrln("Branch is being created. Use 'tiger branch get' to check branch status.")
			} else {
				cmd.PrintErrf("Waiting for branch to be ready (timeout: %v)...\n", waitTimeout)
				if waitErr = common.WaitForBranch(cmd.Context(), common.WaitForBranchArgs{
					Client:       client,
					ProjectID:    projectID,
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
				}
			}

			if err := outputBranch(cmd, cfg, branch, cfg.Output); err != nil {
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
	cmd.Flags().BoolVar(&noSetDefault, "no-set-default", false, "Don't set this branch as the default")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for the branch to be ready")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")

	registerFlagCompletion(cmd, "output", outputCompletion("env"))
	registerFlagCompletion(cmd, "cpu", branchCPUCompletion)
	registerFlagCompletion(cmd, "memory", branchMemoryCompletion)

	return cmd
}
