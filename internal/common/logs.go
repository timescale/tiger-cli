package common

import (
	"context"
	"errors"
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

	// Since and Until bound the logs fetched. A zero Since fetches as far back
	// as Tail allows, and a zero Until fetches up to the current time.
	Since time.Time
	Until time.Time

	// Node selects a specific service node to fetch logs from, for services
	// with HA replicas. If nil, the backend returns logs for the primary.
	Node *int
}

// FetchServiceLogs fetches service logs with cursor-based pagination up to the specified
// tail limit. Returns entries in ascending order by timestamp (oldest first, newest last).
func FetchServiceLogs(ctx context.Context, args FetchServiceLogsArgs) ([]api.ServiceLogEntry, error) {
	params := &api.GetServiceLogsParams{
		Node:  args.Node,
		Since: optionalTime(args.Since),
		Until: fixedUntil(args.Until),
	}

	return fetchLogs(args.Tail, func(cursor *string) ([]api.ServiceLogEntry, *string, error) {
		params.Cursor = cursor
		resp, err := args.Client.GetServiceLogsWithResponse(ctx, args.ProjectID, args.ServiceID, params)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch logs: %w", err)
		}
		if resp.StatusCode() != http.StatusOK {
			return nil, nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
		}
		if resp.JSON200 == nil {
			return nil, nil, errors.New("unexpected empty response")
		}
		var entries []api.ServiceLogEntry
		if resp.JSON200.Entries != nil {
			entries = *resp.JSON200.Entries
		}
		return entries, resp.JSON200.LastCursor, nil
	})
}

type FetchBranchLogsArgs struct {
	Client    api.ClientWithResponsesInterface
	ProjectID string
	// ServiceID is the branch's parent service, which the endpoint is nested under.
	ServiceID string
	BranchID  string
	Tail      int

	// Since and Until bound the logs fetched. A zero Since fetches as far back
	// as Tail allows, and a zero Until fetches up to the current time.
	Since time.Time
	Until time.Time
}

// FetchBranchLogs is FetchServiceLogs for a branch.
func FetchBranchLogs(ctx context.Context, args FetchBranchLogsArgs) ([]api.ServiceLogEntry, error) {
	params := &api.GetBranchLogsParams{
		Since: optionalTime(args.Since),
		Until: fixedUntil(args.Until),
	}

	return fetchLogs(args.Tail, func(cursor *string) ([]api.ServiceLogEntry, *string, error) {
		params.Cursor = cursor
		resp, err := args.Client.GetBranchLogsWithResponse(ctx, args.ProjectID, args.ServiceID, args.BranchID, params)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch logs: %w", err)
		}
		if resp.StatusCode() != http.StatusOK {
			return nil, nil, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
		}
		if resp.JSON200 == nil {
			return nil, nil, errors.New("unexpected empty response")
		}
		return resp.JSON200.Entries, resp.JSON200.LastCursor, nil
	})
}

// optionalTime returns a pointer to t, or nil if t is zero.
func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// fixedUntil returns a pointer to until, or to the current time if until is
// zero. Fixing the upper time bound makes every paginated request share the
// same window — without it, a clock tick between requests could cause the
// second page to return logs already included on the first page. UTC is the
// canonical form for the query parameter it becomes.
func fixedUntil(until time.Time) *time.Time {
	if until.IsZero() {
		return new(time.Now().UTC())
	}
	return &until
}

// fetchLogs pages through logs with fetchPage, which takes the cursor for the
// page (nil for the first) and returns its entries newest-first along with
// the next page's cursor (nil when there are none). It stops once it has tail
// entries, and returns them oldest-first. With no entries, the result is an
// empty slice rather than nil, so it serializes as [] rather than null.
func fetchLogs(tail int, fetchPage func(cursor *string) ([]api.ServiceLogEntry, *string, error)) ([]api.ServiceLogEntry, error) {
	entries := []api.ServiceLogEntry{}
	var cursor *string
	for {
		page, next, err := fetchPage(cursor)
		if err != nil {
			return nil, err
		}
		entries = append(entries, page...)

		// Stop when we have enough logs or the server signals no further pages.
		if len(entries) >= tail || next == nil {
			break
		}
		cursor = next
	}

	// Trim to the requested tail count.
	if len(entries) > tail {
		entries = entries[:tail]
	}

	// Reverse: the API returns logs newest-first; terminal output is oldest-first.
	slices.Reverse(entries)

	return entries, nil
}
