## tiger service metrics details

Get metric details

### Synopsis

Get descriptive metadata for a metric: what it measures, its type, default
aggregation function, and available labels.

Use 'tiger service metrics available-series' to discover valid metric names,
then 'tiger service metrics series' to fetch its data.

These metrics have no richer metadata — expect just the name back, with type,
default aggregation, description, and labels all empty: timescale_cloud_system_cpu_total_millicores, timescale_cloud_system_cpu_usage_millicores, timescale_cloud_system_disk_io_read_bytes, timescale_cloud_system_disk_io_read_ops, timescale_cloud_system_disk_io_total_bytes, timescale_cloud_system_disk_io_total_ops, timescale_cloud_system_disk_io_write_bytes, timescale_cloud_system_disk_io_write_ops, timescale_cloud_system_disk_usage_bytes, timescale_cloud_system_memory_total_bytes, timescale_cloud_system_memory_usage_bytes, timescale_cloud_database_qps, timescale_cloud_database_num_connections, timescale_cloud_database_job_duration_usecs, timescale_cloud_database_job_success.

```
tiger service metrics details [service-id] [flags]
```

### Examples

```
  # Describe a metric
  tiger service metrics details --metric pg_stat_activity_count

  # Get metric details as JSON
  tiger service metrics details --metric pg_stat_activity_count --output json
```

### Options

```
  -h, --help            help for details
      --metric string   Metric name
  -o, --output string   Output format (json, yaml, table)
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
