package common

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
)

type FetchServiceLogsArgs struct {
	Client    api.ClientWithResponsesInterface
	ProjectID string
	ServiceID string
	Tail      int
	Since     *time.Time
	Until     *time.Time

	// Node selects a specific service node to fetch logs from, for services
	// with HA replicas. If nil, the backend returns logs for the primary.
	Node *int
}

// FetchServiceLogs fetches service logs with cursor-based pagination up to the specified
// tail limit. Returns entries in ascending order by timestamp (oldest first, newest last).
func FetchServiceLogs(ctx context.Context, args FetchServiceLogsArgs) ([]api.ServiceLogEntry, error) {
	params := &api.GetServiceLogsParams{
		Node:  args.Node,
		Since: args.Since,
		Until: args.Until,
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
		resp, err := args.Client.GetServiceLogsWithResponse(ctx, args.ProjectID, args.ServiceID, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch logs: %w", err)
		}

		if resp.StatusCode() != http.StatusOK {
			return nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
		}

		if resp.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response")
		}

		if resp.JSON200.Entries != nil {
			entries = append(entries, *resp.JSON200.Entries...)
		}

		// Stop when we have enough logs or the server signals no further pages.
		if len(entries) >= args.Tail || resp.JSON200.LastCursor == nil {
			break
		}

		params.Cursor = resp.JSON200.LastCursor
	}

	// Trim to the requested tail count.
	if len(entries) > args.Tail {
		entries = entries[:args.Tail]
	}

	// Reverse: the API returns logs newest-first; terminal output is oldest-first.
	slices.Reverse(entries)

	return entries, nil
}
