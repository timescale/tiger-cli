package cmd

import (
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsSeriesCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	fromTime := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	toTime := time.Date(2026, 5, 13, 1, 0, 0, 0, time.UTC)

	args := func(filter string) []string {
		return []string{
			"service", "metrics", "series", "svc-12345",
			"--metric", "some_metric",
			"--from", "2026-05-13T00:00:00Z",
			"--to", "2026-05-13T01:00:00Z",
			"--filter", filter,
		}
	}

	// expectSeries registers the mock for the given filters, returning an
	// empty result — the test only cares about the request body sent.
	expectSeries := func(filters []api.MetricLabelFilter) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			empty := []api.MetricSeries{}
			m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "svc-12345", api.MetricsSeriesRequest{
				Name:    "some_metric",
				From:    fromTime,
				To:      toTime,
				Filters: &filters,
			}).Return(&api.GetServiceMetricsSeriesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &empty,
			}, nil)
		}
	}

	const noDataMsg = "No metric data returned for the requested window.\n"

	runCmdTests(t, []cmdTest{
		{
			name:       "key=value filter builds an EQUAL request",
			args:       args("ordinal=0"),
			opts:       []runOption{experimental},
			setup:      expectSeries([]api.MetricLabelFilter{{Key: "ordinal", Value: "0"}}),
			wantStdout: noDataMsg,
		},
		{
			name: "key!=value filter builds a NOT_EQUAL request",
			args: args("role!=replica"),
			opts: []runOption{experimental},
			setup: expectSeries([]api.MetricLabelFilter{
				{Key: "role", Value: "replica", MatchType: new(api.MetricMatchTypeNOTEQUAL)},
			}),
			wantStdout: noDataMsg,
		},
		{
			name:    "malformed filter",
			args:    args("nokeyvalue"),
			opts:    []runOption{experimental},
			wantErr: `--filter must be name=value or name!=value, got "nokeyvalue"`,
		},
		{
			name:    "filter missing a value after !=",
			args:    args("role!="),
			opts:    []runOption{experimental},
			wantErr: `--filter must be name=value or name!=value, got "role!="`,
		},
	})
}
