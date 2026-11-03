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

// buildServiceResizeCmd creates the resize subcommand
func buildServiceResizeCmd(app *common.App) *cobra.Command {
	var cpu string
	var memory string
	var iops int

	// IOPS are a preview, so their help text is shown only behind the
	// experimental gate.
	var iopsDescHelp, iopsSectionHelp, iopsExampleHelp string
	if app.Experimental {
		iopsDescHelp = ", memory, and provisioned IOPS\n"
		iopsSectionHelp = `

The allowed --iops values depend on your plan, the service's platform, and its
CPU/memory allocation. See the Tiger Data documentation for details. If only
--iops is given, the service keeps its current CPU and memory. If --cpu or
--memory is given without --iops on the fluid platform, the IOPS is reset to
the baseline for the new size.`
		iopsExampleHelp = "\n\n  # Change only the provisioned IOPS\n  tiger service resize --iops 16000"
	} else {
		iopsDescHelp = " and memory "
	}

	cmd := &cobra.Command{
		Use:   "resize [name-or-id]",
		Short: "Resize a database service",
		Long: `Resize a database service by changing its CPU` + iopsDescHelp + `allocation.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. This command changes the compute and memory
resources allocated to your database service.

The command returns once the resize request is accepted, without waiting for
the resize to complete, which may take several minutes. The service may be
temporarily unavailable during the resize operation. Note that changing
resources will affect your billing - increasing resources will increase costs.

` + cpuMemoryHelp(false, app.Experimental) + `

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).` + iopsSectionHelp,
		Example: `  # Resize default service to 2 CPU cores and 8GB memory
  tiger service resize --cpu 2000 --memory 8

  # Resize specific service to 4 CPU cores and 16GB memory
  tiger service resize svc-12345 --cpu 4000 --memory 16

  # Resize service using only CPU (memory will be auto-configured to 8GB)
  tiger service resize --cpu 2000

  # Resize service using only memory (CPU will be auto-configured to 4000m)
  tiger service resize --memory 16` + iopsExampleHelp,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			warnResizeWaitFlags(cmd)

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Determine the service ref
			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}

			// --iops only exists behind the experimental gate
			iopsSet := app.Experimental && cmd.Flags().Changed("iops")

			if cpu == "" && memory == "" && !iopsSet {
				if app.Experimental {
					return errors.New("must specify at least one of --cpu, --memory, or --iops")
				}
				return errors.New("must specify at least one of --cpu or --memory")
			}

			resolved, err := resolveServiceForWrite(cmd.Context(), cfg, client, projectID, serviceRef)
			if err != nil {
				return err
			}

			// Validate against the combinations of the service's platform. Not
			// gated on experimental: only a service already created on the
			// fluid platform gets the fluid combinations.
			cpuMemoryCfg, err := common.CPUMemoryConfigsFor(util.Deref(resolved.Platform)).WithoutShared().ValidateAndNormalize(cpu, memory)
			if err != nil {
				return err
			}

			// Prepare resize request. An IOPS-only resize omits CPU and memory,
			// which the API leaves unchanged.
			resizeReq := api.ResizeInput{
				CPUMillis: cpuMemoryCfg.CPUMillisString(),
				MemoryGbs: cpuMemoryCfg.MemoryGBsString(),
			}
			var targets []string
			if cpuMemoryCfg != nil {
				targets = append(targets, cpuMemoryCfg.String())
			}
			if iopsSet {
				resizeReq.Iops = &iops
				targets = append(targets, fmt.Sprintf("%d IOPS", iops))
			}
			target := strings.Join(targets, ", ")

			// Display resize information
			cmd.PrintErrf("Resizing service %s to %s...\n", serviceLabel(*resolved), target)

			// Make API call to resize service
			resp, err := client.ResizeServiceWithResponse(cmd.Context(), projectID, resolved.ServiceID, resizeReq)
			if err != nil {
				return fmt.Errorf("failed to resize service: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}

			cmd.PrintErrf("Resize request accepted for service %s.\n", serviceLabel(*resolved))
			cmd.PrintErrln(resizeInProgressMsg)
			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in millicores")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes")
	if app.Experimental {
		cmd.Flags().IntVar(&iops, "iops", 0, "Provisioned IOPS")
		registerFlagCompletion(cmd, "iops", iopsCompletion)
	}
	addResizeWaitFlags(cmd)

	registerFlagCompletion(cmd, "cpu", cpuCompletion(false, app.Experimental))
	registerFlagCompletion(cmd, "memory", memoryCompletion(false, app.Experimental))

	return cmd
}
