package common

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// FetchServiceLogs fetches service logs with pagination support up to the specified
// tail limit. Returns logs in ascending order by timestamp (oldest first, newest last).
// NOTE: The node parameter specifies the Specific service node to fetch logs
// from, for services with HA replicas. If nil, the backend automatically
// returns logs for the primary.
func FetchServiceLogs(
	ctx context.Context,
	cfg *Config,
	serviceID string,
	tail int,
	until *time.Time,
	node *int,
) ([]string, error) {
	// Initialize params
	params := &api.GetServiceLogsParams{
		Node:  node,
		Page:  util.Ptr(0),
		Until: until,
	}

	// Set until time to current time if not provided for
	// consistent pagination across multiple page requests
	if params.Until == nil {
		now := time.Now()
		params.Until = &now
	}

	// Fetch pages until we have enough logs or reach the end
	var logs []string
	for {
		resp, err := cfg.Client.GetServiceLogsWithResponse(ctx, cfg.ProjectID, serviceID, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch logs: %w", err)
		}

		if resp.StatusCode() != http.StatusOK {
			return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
		}

		if resp.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response")
		}

		pageLogs := resp.JSON200.Logs
		logs = append(logs, pageLogs...)

		// Stop conditions:
		// 1. Page is empty (no more logs available)
		// 2. We have enough logs to satisfy tail requirement
		if len(pageLogs) == 0 || len(logs) >= tail {
			break
		}

		*params.Page++
	}

	// Apply tail filter
	if len(logs) > tail {
		logs = logs[:tail]
	}

	// Reverse the order of the logs. This is necessary because the API
	// returns logs in descending order by timestamp (with the most
	// recent logs first), whereas in terminal output, it's more natural
	// for the most recent logs to appear last.
	slices.Reverse(logs)

	return logs, nil
}
