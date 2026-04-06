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
// tail limit. Returns entries in ascending order by timestamp (oldest first, newest last).
// NOTE: The node parameter specifies the specific service node to fetch logs
// from, for services with HA replicas. If nil, the backend automatically
// returns logs for the primary.
func FetchServiceLogs(
	ctx context.Context,
	cfg *Config,
	serviceID string,
	tail int,
	from *time.Time,
	until *time.Time,
	node *int,
) ([]api.ServiceLogEntry, error) {
	params := &api.GetServiceLogsParams{
		Node:  node,
		From:  from,
		Until: until,
	}

	// Fix the upper time bound so that all paginated requests share the same
	// window — without this, a clock tick between requests could cause the
	// second page to return logs already included on the first page.
	if params.Until == nil {
		now := time.Now()
		params.Until = &now
	}

	var entries []api.ServiceLogEntry
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

		page := toEntries(resp.JSON200)
		entries = append(entries, page...)

		// Stop when we have enough logs or the server signals no further pages.
		if len(entries) >= tail || resp.JSON200.LastCursor == nil {
			break
		}

		params.Cursor = resp.JSON200.LastCursor
	}

	// Trim to the requested tail count.
	if len(entries) > tail {
		entries = entries[:tail]
	}

	// Reverse: the API returns logs newest-first; terminal output is oldest-first.
	slices.Reverse(entries)

	return entries, nil
}

// toEntries converts a ServiceLogs response to a slice of ServiceLogEntry.
// Uses the structured entries field when available (cursor path); falls back
// to constructing minimal entries from the plain logs strings (legacy path).
func toEntries(logs *api.ServiceLogs) []api.ServiceLogEntry {
	if logs.Entries != nil {
		return *logs.Entries
	}
	entries := make([]api.ServiceLogEntry, len(logs.Logs))
	for i, msg := range logs.Logs {
		entries[i] = api.ServiceLogEntry{Message: msg}
	}
	return entries
}
