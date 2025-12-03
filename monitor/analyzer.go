package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// MetricStats captures the statistical signature for one metric.
type MetricStats struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
}

// BaselineSpec contains the computed normal-behavior signature.
type BaselineSpec struct {
	GeneratedAt    time.Time              `json:"generated_at"`
	SampleCount    int                    `json:"sample_count"`
	IntervalMillis int                    `json:"interval_millis"`
	Metrics        map[string]MetricStats `json:"metrics"`
}

// Anomaly represents a single deviation beyond the baseline threshold.
type Anomaly struct {
	Metric      string    `json:"metric"`
	Timestamp   time.Time `json:"timestamp"`
	SampleIdx   int       `json:"sample_index"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Direction   string    `json:"direction"`
	StdDevsAway float64   `json:"std_devs_away"`
}

// AnomalyReport aggregates detection results for a dataset.
type AnomalyReport struct {
	GeneratedAt      time.Time        `json:"generated_at"`
	Sensitivity      float64          `json:"sensitivity"`
	BaselineSummary  BaselineSpec     `json:"baseline"`
	SnapshotMetadata SnapshotMetadata `json:"snapshot_metadata"`
	Anomalies        []Anomaly        `json:"anomalies"`
}

var metricExtractors = map[string]func(MetricSample) float64{
	"cpu_percent":       func(s MetricSample) float64 { return s.CPUPercent },
	"memory_percent":    func(s MetricSample) float64 { return s.MemoryPercent },
	"memory_used_mb":    func(s MetricSample) float64 { return s.MemoryUsedMB },
	"disk_used_percent": func(s MetricSample) float64 { return s.DiskUsedPercent },
	"disk_used_gb":      func(s MetricSample) float64 { return s.DiskUsedGB },
}

// ComputeBaseline derives the statistical signature for a snapshot.
func ComputeBaseline(snapshot MetricsSnapshot) BaselineSpec {
	metrics := make(map[string]MetricStats)
	for key, fn := range metricExtractors {
		values := make([]float64, len(snapshot.Samples))
		for i, s := range snapshot.Samples {
			values[i] = fn(s)
		}
		metrics[key] = computeStats(values)
	}
	return BaselineSpec{
		GeneratedAt:    time.Now(),
		SampleCount:    len(snapshot.Samples),
		IntervalMillis: snapshot.Metadata.IntervalMillis,
		Metrics:        metrics,
	}
}

func computeStats(values []float64) MetricStats {
	if len(values) == 0 {
		return MetricStats{}
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	return MetricStats{Mean: mean, StdDev: math.Sqrt(variance)}
}

// SaveBaseline persists a baseline spec to disk.
func SaveBaseline(path string, baseline BaselineSpec) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadBaseline loads a baseline spec from JSON.
func LoadBaseline(path string) (BaselineSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BaselineSpec{}, fmt.Errorf("read baseline: %w", err)
	}
	var baseline BaselineSpec
	if err := json.Unmarshal(data, &baseline); err != nil {
		return BaselineSpec{}, fmt.Errorf("parse baseline: %w", err)
	}
	return baseline, nil
}

// DetectAnomalies compares snapshot data against the baseline signature.
func DetectAnomalies(snapshot MetricsSnapshot, baseline BaselineSpec, sensitivity float64) []Anomaly {
	if sensitivity <= 0 {
		sensitivity = 3
	}
	anomalies := make([]Anomaly, 0)
	for metric, fn := range metricExtractors {
		stats, ok := baseline.Metrics[metric]
		if !ok {
			continue
		}
		upper, lower := thresholds(stats, sensitivity)
		for idx, sample := range snapshot.Samples {
			val := fn(sample)
			if val > upper {
				anomalies = append(anomalies, Anomaly{
					Metric:      metric,
					Timestamp:   sample.Timestamp,
					SampleIdx:   idx,
					Value:       val,
					Threshold:   upper,
					Direction:   "above",
					StdDevsAway: stddevsAway(val, stats.Mean, stats.StdDev),
				})
			} else if val < lower {
				anomalies = append(anomalies, Anomaly{
					Metric:      metric,
					Timestamp:   sample.Timestamp,
					SampleIdx:   idx,
					Value:       val,
					Threshold:   lower,
					Direction:   "below",
					StdDevsAway: stddevsAway(val, stats.Mean, stats.StdDev),
				})
			}
		}
	}
	return anomalies
}

func thresholds(stats MetricStats, sensitivity float64) (float64, float64) {
	span := stats.StdDev
	if span == 0 {
		span = math.Max(stats.Mean*0.05, 0.5)
	}
	delta := sensitivity * span
	return stats.Mean + delta, stats.Mean - delta
}

func stddevsAway(val, mean, stddev float64) float64 {
	if stddev <= 0 {
		return 0
	}
	return math.Abs(val-mean) / stddev
}

// SaveAnomalyReport writes the anomaly report to disk.
func SaveAnomalyReport(path string, report AnomalyReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal anomaly report: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
