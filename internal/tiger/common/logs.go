package common

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

// FetchServiceLogs fetches service logs with cursor-based pagination up to the specified
// tail limit. Returns logs in ascending order by timestamp (oldest first, newest last).
// NOTE: The node parameter specifies the specific service node to fetch logs
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
	params := &api.GetServiceLogsParams{
		Node:  node,
		Until: until,
	}

	// Fix the upper time bound so that all paginated requests share the same
	// window — without this, a clock tick between requests could cause the
	// second page to return logs already included on the first page.
	if params.Until == nil {
		now := time.Now()
		params.Until = &now
	}

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

		logs = append(logs, resp.JSON200.Logs...)

		// Stop when we have enough logs or the server signals no further pages.
		if len(logs) >= tail || resp.JSON200.LastCursor == nil {
			break
		}

		params.Cursor = resp.JSON200.LastCursor
	}

	// Trim to the requested tail count.
	if len(logs) > tail {
		logs = logs[:tail]
	}

	// Reverse: the API returns logs newest-first; terminal output is oldest-first.
	slices.Reverse(logs)

	return logs, nil
}
