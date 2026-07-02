package util

import (
	"sort"
	"time"
)

// MetricPoint is a single data point used for summary computation.
type MetricPoint struct {
	Time  time.Time
	Value *float64
}

// MetricSummary holds aggregated statistics for one labeled metric series over a time window.
type MetricSummary struct {
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels,omitempty"`
	From    time.Time         `json:"from"`
	To      time.Time         `json:"to"`
	Count   int               `json:"count"`
	Min     float64           `json:"min"`
	MinTime time.Time         `json:"min_time"`
	Max     float64           `json:"max"`
	MaxTime time.Time         `json:"max_time"`
	Avg     float64           `json:"avg"`
	P50     float64           `json:"p50"`
	P95     float64           `json:"p95"`
}

// SummarizeMetrics computes aggregated stats from a slice of data points.
// Points with nil values are skipped. Returns nil when no non-nil points exist.
func SummarizeMetrics(name string, labels map[string]string, points []MetricPoint, from, to time.Time) *MetricSummary {
	var vals []float64
	var times []time.Time
	for _, p := range points {
		if p.Value == nil {
			continue
		}
		vals = append(vals, *p.Value)
		times = append(times, p.Time)
	}
	if len(vals) == 0 {
		return nil
	}

	minVal, maxVal := vals[0], vals[0]
	minTime, maxTime := times[0], times[0]
	var sum float64
	for i, v := range vals {
		sum += v
		if v < minVal {
			minVal = v
			minTime = times[i]
		}
		if v > maxVal {
			maxVal = v
			maxTime = times[i]
		}
	}

	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	return &MetricSummary{
		Name:    name,
		Labels:  labels,
		From:    from,
		To:      to,
		Count:   len(vals),
		Min:     minVal,
		MinTime: minTime,
		Max:     maxVal,
		MaxTime: maxTime,
		Avg:     sum / float64(len(vals)),
		P50:     percentile(sorted, 50),
		P95:     percentile(sorted, 95),
	}
}

// DefaultBucketSeconds returns a sensible bucket size for the given time window.
// Windows of one hour or less use per-minute buckets; longer windows use per-hour.
func DefaultBucketSeconds(from, to time.Time) int {
	if to.Sub(from) <= time.Hour {
		return 60
	}
	return 3600
}

// percentile returns the p-th percentile of a pre-sorted slice using nearest-rank.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int((p / 100.0) * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
