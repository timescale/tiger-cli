## tiger service metrics series

Get metric series data

### Synopsis

Get time-series data for a specific metric.

Use 'tiger service metrics available' to discover valid metric names.

Each labeled series (e.g. one per replica) is returned independently with its
full list of raw data points.

--from and --to default to the last 24 hours when both are omitted, bucketed
into 1-hour (3600s) intervals unless --bucket-seconds is also given.

```
tiger service metrics series [service-id] [flags]
```

### Examples

```
  # Fetch CPU usage for the last 24 hours (the default window)
  tiger service metrics series --metric timescale_cloud_system_cpu_usage_millicores

  # Fetch CPU usage for a specific hour
  tiger service metrics series --metric timescale_cloud_system_cpu_usage_millicores \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z

  # Get memory data points as JSON
  tiger service metrics series --metric timescale_cloud_system_memory_usage_bytes \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z --output json

  # Fetch data for the primary instance only
  tiger service metrics series --metric timescale_cloud_system_cpu_usage_millicores \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z --role PRIMARY

  # Filter by an arbitrary label
  tiger service metrics series --metric some_metric_name \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z \
    --filter ordinal=0

  # Exclude a label value
  tiger service metrics series --metric some_metric_name \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z \
    --filter role!=replica

  # Break the result into one series per role
  tiger service metrics series --metric some_metric_name \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z \
    --group-by role
```

### Options

```
      --bucket-seconds int   Aggregation bucket size in seconds (minimum 60s). Defaults to 3600 (1h) when --from/--to are also omitted; otherwise the server auto-selects based on the time window
      --filter strings       Arbitrary label filter as name=value or name!=value (repeatable)
      --fn string            Aggregation function applied per bucket. One of: RATE, INCREASE, SUM, AVG, MIN, MAX, MIN_TOTAL, MAX_TOTAL, COUNT, P50, P90, P99, LAST. Rejected on the timescale_cloud_* resource/qps/connections/jobs metrics; omit to let the server pick the default
      --from string          Start of the time window (RFC3339). Defaults to 24 hours ago when --to is also omitted
      --group-by strings     Label key to break the result into one series per distinct value (repeatable). Rejected on the same metrics that reject --fn; omit to collapse into a single series
  -h, --help                 help for series
      --metric string        Metric series name
  -o, --output string        Output format (json, yaml, table)
      --role string          Filter to a specific instance role (PRIMARY or REPLICA)
      --to string            End of the time window (RFC3339). Defaults to now when --from is also omitted
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --color                     enable colored output (default true)
      --config-dir string         config directory (default "~/.config/tiger")
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --service-id string         service ID
      --version-check             check for updates on startup (default true)
```

### SEE ALSO

* [tiger service metrics](tiger_service_metrics.md)	 - View service metrics
