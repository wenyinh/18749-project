package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// MetricSample represents one snapshot of OS-level metrics.
type MetricSample struct {
	Timestamp       time.Time `json:"timestamp"`
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryPercent   float64   `json:"memory_percent"`
	MemoryUsedMB    float64   `json:"memory_used_mb"`
	DiskUsedPercent float64   `json:"disk_used_percent"`
	DiskUsedGB      float64   `json:"disk_used_gb"`
}

// SnapshotMetadata captures metadata describing a collection run.
type SnapshotMetadata struct {
	IntervalMillis  int       `json:"interval_millis"`
	SampleCount     int       `json:"sample_count"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	DiskPath        string    `json:"disk_path"`
	Hostname        string    `json:"hostname"`
	FaultInjected   string    `json:"fault_injected,omitempty"`
}

// MetricsSnapshot wraps the collected samples and metadata.
type MetricsSnapshot struct {
	Metadata SnapshotMetadata `json:"metadata"`
	Samples  []MetricSample   `json:"samples"`
}

// CollectOptions configure a collection run.
type CollectOptions struct {
	Interval      time.Duration
	Duration      time.Duration
	DiskPath      string
	FaultType     string
	FaultDelay    time.Duration
	FaultDuration time.Duration
}

// Collector periodically polls OS metrics.
type Collector struct {
	interval     time.Duration
	diskPath     string
	prevCPUTimes *cpu.TimesStat
}

// NewCollector creates a Collector for the provided interval and disk path.
func NewCollector(interval time.Duration, diskPath string) *Collector {
	if interval <= 0 {
		interval = time.Second
	}
	if diskPath == "" {
		diskPath = "/"
	}
	return &Collector{interval: interval, diskPath: diskPath}
}

// Collect gathers metrics until the duration elapses or the context is cancelled.
func (c *Collector) Collect(ctx context.Context, duration time.Duration) ([]MetricSample, error) {
	if duration <= 0 {
		return nil, errors.New("duration must be > 0")
	}

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	timer := time.NewTimer(duration)
	defer timer.Stop()

	samples := make([]MetricSample, 0, int(duration/c.interval)+1)

	// Collect immediately so baseline includes t=0.
	first, err := c.sample()
	if err != nil {
		return nil, err
	}
	samples = append(samples, first)

	for {
		select {
		case <-ctx.Done():
			return samples, ctx.Err()
		case <-timer.C:
			return samples, nil
		case <-ticker.C:
			s, err := c.sample()
			if err != nil {
				return samples, err
			}
			samples = append(samples, s)
		}
	}
}

func (c *Collector) sample() (MetricSample, error) {
	now := time.Now()

	cpuPercent, err := c.cpuUsagePercent()
	if err != nil {
		return MetricSample{}, err
	}

	memStats, err := mem.VirtualMemory()
	if err != nil {
		return MetricSample{}, err
	}

	diskUsage, err := disk.Usage(c.diskPath)
	if err != nil {
		return MetricSample{}, err
	}

	sample := MetricSample{
		Timestamp:       now,
		CPUPercent:      cpuPercent,
		MemoryPercent:   memStats.UsedPercent,
		MemoryUsedMB:    bytesToMB(memStats.Used),
		DiskUsedPercent: diskUsage.UsedPercent,
		DiskUsedGB:      bytesToGB(diskUsage.Used),
	}
	return sample, nil
}

func (c *Collector) cpuUsagePercent() (float64, error) {
	stats, err := cpu.Times(false)
	if err != nil {
		return 0, err
	}
	if len(stats) == 0 {
		return 0, errors.New("cpu stats unavailable")
	}
	cur := stats[0]
	if c.prevCPUTimes == nil {
		copy := cur
		c.prevCPUTimes = &copy
		return 0, nil
	}

	prev := c.prevCPUTimes
	totalDelta := cur.Total() - prev.Total()
	if totalDelta <= 0 {
		copy := cur
		c.prevCPUTimes = &copy
		return 0, nil
	}
	idleDelta := (cur.Idle + cur.Iowait) - (prev.Idle + prev.Iowait)
	usage := 100 * (1 - idleDelta/totalDelta)
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	copy := cur
	c.prevCPUTimes = &copy
	if math.IsNaN(usage) || math.IsInf(usage, 0) {
		return 0, nil
	}
	return usage, nil
}

func bytesToMB(v uint64) float64 {
	return float64(v) / (1024 * 1024)
}

func bytesToGB(v uint64) float64 {
	return float64(v) / (1024 * 1024 * 1024)
}

// RunCollection orchestrates a full collection workflow and returns the snapshot.
func RunCollection(ctx context.Context, opts CollectOptions) (MetricsSnapshot, error) {
	if opts.Duration <= 0 {
		return MetricsSnapshot{}, errors.New("duration must be > 0")
	}
	collector := NewCollector(opts.Interval, opts.DiskPath)

	hostname, _ := os.Hostname()

	collectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opts.FaultType != "" {
		startFaultInjection(collectionCtx, opts)
	}

	samples, err := collector.Collect(collectionCtx, opts.Duration)
	if err != nil && !errors.Is(err, context.Canceled) {
		return MetricsSnapshot{}, err
	}
	if len(samples) == 0 {
		return MetricsSnapshot{}, errors.New("no samples collected")
	}

	start := samples[0].Timestamp
	end := samples[len(samples)-1].Timestamp

	metadata := SnapshotMetadata{
		IntervalMillis:  int(collector.interval / time.Millisecond),
		SampleCount:     len(samples),
		StartTime:       start,
		EndTime:         end,
		DurationSeconds: end.Sub(start).Seconds(),
		DiskPath:        collector.diskPath,
		Hostname:        hostname,
	}

	if opts.FaultType != "" {
		metadata.FaultInjected = describeFault(opts)
	}

	return MetricsSnapshot{Metadata: metadata, Samples: samples}, nil
}

// SaveSnapshot writes the snapshot to disk as pretty JSON.
func SaveSnapshot(path string, snapshot MetricsSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadSnapshot loads a snapshot JSON file.
func LoadSnapshot(path string) (MetricsSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read snapshot: %w", err)
	}
	var snap MetricsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("parse snapshot: %w", err)
	}
	return snap, nil
}
