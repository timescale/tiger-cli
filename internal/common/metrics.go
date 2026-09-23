package common

// LegacyMetrics lists the resource/qps/connections/jobs metrics that predate
// the metrics registry (savannah-gateway's legacySeries). Requests with fn or
// group_by against one of these are rejected with INVALID_REQUEST, and
// service_metrics_details returns no type, default aggregation, description,
// or labels for them — just the name.
var LegacyMetrics = []string{
	"timescale_cloud_system_cpu_total_millicores",
	"timescale_cloud_system_cpu_usage_millicores",
	"timescale_cloud_system_disk_io_read_bytes",
	"timescale_cloud_system_disk_io_read_ops",
	"timescale_cloud_system_disk_io_total_bytes",
	"timescale_cloud_system_disk_io_total_ops",
	"timescale_cloud_system_disk_io_write_bytes",
	"timescale_cloud_system_disk_io_write_ops",
	"timescale_cloud_system_disk_usage_bytes",
	"timescale_cloud_system_memory_total_bytes",
	"timescale_cloud_system_memory_usage_bytes",
	"timescale_cloud_database_qps",
	"timescale_cloud_database_num_connections",
	"timescale_cloud_database_job_duration_usecs",
	"timescale_cloud_database_job_success",
}
