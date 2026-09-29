package cmd

import (
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsSeriesCmd(t *testing.T) {
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
				MetricName: "some_metric",
				From:       fromTime,
				To:         toTime,
				Filters:    &filters,
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
			mock:       expectSeries([]api.MetricLabelFilter{{Key: "ordinal", Value: "0"}}),
			wantStdout: noDataMsg,
		},
		{
			name: "key!=value filter builds a NOT_EQUAL request",
			args: args("role!=replica"),
			mock: expectSeries([]api.MetricLabelFilter{
				{Key: "role", Value: "replica", MatchType: new(api.MetricMatchTypeNOTEQUAL)},
			}),
			wantStdout: noDataMsg,
		},
		{
			name:    "malformed filter",
			args:    args("nokeyvalue"),
			wantErr: `--filter must be name=value or name!=value, got "nokeyvalue"`,
		},
		{
			name:    "filter missing a value after !=",
			args:    args("role!="),
			wantErr: `--filter must be name=value or name!=value, got "role!="`,
		},
		{
			name: "group-by builds a GroupBy request",
			args: []string{
				"service", "metrics", "series", "svc-12345",
				"--metric", "some_metric",
				"--from", "2026-05-13T00:00:00Z",
				"--to", "2026-05-13T01:00:00Z",
				"--group-by", "role",
				"--group-by", "ordinal",
			},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				empty := []api.MetricSeries{}
				groupBy := []string{"role", "ordinal"}
				m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "svc-12345", api.MetricsSeriesRequest{
					MetricName: "some_metric",
					From:       fromTime,
					To:         toTime,
					GroupBy:    &groupBy,
				}).Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &empty,
				}, nil)
			},
			wantStdout: noDataMsg,
		},
		{
			// synctest's bubble clock always starts at 2000-01-01 UTC, so the
			// default window and bucket size can be spelled out exactly.
			name:     "omitting --from and --to defaults to the last 24 hours at a 1h bucket",
			args:     []string{"service", "metrics", "series", "svc-12345", "--metric", "some_metric"},
			synctest: true,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				empty := []api.MetricSeries{}
				bucket := 3600
				m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "svc-12345", api.MetricsSeriesRequest{
					MetricName:    "some_metric",
					From:          time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC),
					To:            time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					BucketSeconds: &bucket,
				}).Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &empty,
				}, nil)
			},
			wantStdout: noDataMsg,
		},
		{
			// Defaulting only applies when both --from and --to are omitted —
			// giving just one still requires its counterpart.
			name:    "omitting only --to still requires it",
			args:    []string{"service", "metrics", "series", "svc-12345", "--metric", "some_metric", "--from", "2026-05-13T00:00:00Z"},
			wantErr: `--to must be RFC3339 (e.g., 2026-05-13T01:00:00Z): parsing time "" as "2006-01-02T15:04:05Z07:00": cannot parse "" as "2006"`,
		},
		{
			// An explicit --bucket-seconds is respected even in the default
			// window, rather than being overridden by the 3600s default.
			name:     "explicit --bucket-seconds overrides the default window's bucket size",
			args:     []string{"service", "metrics", "series", "svc-12345", "--metric", "some_metric", "--bucket-seconds", "60"},
			synctest: true,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				empty := []api.MetricSeries{}
				bucket := 60
				m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "svc-12345", api.MetricsSeriesRequest{
					MetricName:    "some_metric",
					From:          time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC),
					To:            time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					BucketSeconds: &bucket,
				}).Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &empty,
				}, nil)
			},
			wantStdout: noDataMsg,
		},
	})
}
