package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// Sample captures a single snapshot of black-box metrics.
type Sample struct {
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemPercent  float64   `json:"mem_percent"`
	MemUsedMB   float64   `json:"mem_used_mb"`
	DiskPercent float64   `json:"disk_percent"`
	Anomaly     bool      `json:"anomaly,omitempty"`
	Reasons     []string  `json:"reasons,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// MeanStd summarizes baseline statistics for a metric.
type MeanStd struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"stddev"`
}

// BaselineSignature represents "normal" behavior derived from the baseline window.
type BaselineSignature struct {
	Count   int     `json:"count"`
	CPU     MeanStd `json:"cpu"`
	Memory  MeanStd `json:"memory"`
	Disk    MeanStd `json:"disk"`
	Comment string  `json:"comment,omitempty"`
}

// Report aggregates samples plus metadata for visualization.
type Report struct {
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	IntervalMs    int               `json:"interval_ms"`
	DurationSec   float64           `json:"duration_sec"`
	Threshold     float64           `json:"threshold"`
	FaultInjected string            `json:"fault_injected,omitempty"`
	Baseline      BaselineSignature `json:"baseline"`
	Samples       []Sample          `json:"samples"`
}

// RunningStat tracks running mean/std without storing all values.
type RunningStat struct {
	count int
	mean  float64
	m2    float64
}

func (r *RunningStat) Add(v float64) {
	r.count++
	delta := v - r.mean
	r.mean += delta / float64(r.count)
	r.m2 += delta * (v - r.mean)
}

func (r *RunningStat) Count() int {
	return r.count
}

func (r *RunningStat) Mean() float64 {
	return r.mean
}

func (r *RunningStat) StdDev() float64 {
	if r.count < 2 {
		return 0
	}
	return math.Sqrt(r.m2 / float64(r.count-1))
}

func (r *RunningStat) Summary() MeanStd {
	return MeanStd{
		Mean:   r.Mean(),
		StdDev: r.StdDev(),
	}
}

// Collector manages baseline learning, anomaly detection, and sample storage.
type Collector struct {
	startTime      time.Time
	baselineUntil  time.Time
	threshold      float64
	cpuStats       RunningStat
	memStats       RunningStat
	diskStats      RunningStat
	baselineReady  bool
	signature      BaselineSignature
	samples        []Sample
	baselineFrozen bool
	mu             sync.RWMutex
}

func NewCollector(start time.Time, baselineDuration time.Duration, threshold float64) *Collector {
	until := start.Add(baselineDuration)
	if baselineDuration <= 0 {
		until = start
	}
	return &Collector{
		startTime:     start,
		baselineUntil: until,
		threshold:     threshold,
		samples:       make([]Sample, 0),
	}
}

// CollectSample queries the OS for CPU, memory, and disk usage.
func CollectSample() (Sample, error) {
	now := time.Now()

	cpuPercents, err := cpu.Percent(0, false)
	cpuPct := 0.0
	if err == nil && len(cpuPercents) > 0 {
		cpuPct = cpuPercents[0]
	}

	vm, err := mem.VirtualMemory()
	memPct := 0.0
	memMB := 0.0
	if err == nil {
		memPct = vm.UsedPercent
		memMB = float64(vm.Used) / (1024.0 * 1024.0)
	}

	diskStat, err := disk.Usage("/")
	diskPct := 0.0
	if err == nil {
		diskPct = diskStat.UsedPercent
	}

	return Sample{
		Timestamp:   now,
		CPUPercent:  cpuPct,
		MemPercent:  memPct,
		MemUsedMB:   memMB,
		DiskPercent: diskPct,
	}, nil
}

// Process ingests a sample, updating baseline stats or marking anomalies.
func (c *Collector) Process(sample Sample, now time.Time) Sample {
	c.mu.Lock()
	if !c.baselineReady {
		c.cpuStats.Add(sample.CPUPercent)
		c.memStats.Add(sample.MemPercent)
		c.diskStats.Add(sample.DiskPercent)
		sample.Note = "baseline"

		if !now.Before(c.baselineUntil) {
			c.baselineReady = true
			c.freezeBaselineLocked("baseline window complete")
		}
	} else {
		if !c.baselineFrozen {
			c.freezeBaselineLocked("baseline frozen with limited samples")
		}
		sample = c.flagAnomaliesLocked(sample)
	}

	c.samples = append(c.samples, sample)
	c.mu.Unlock()
	return sample
}

func (c *Collector) freezeBaseline(comment string) {
	c.mu.Lock()
	c.freezeBaselineLocked(comment)
	c.mu.Unlock()
}

func (c *Collector) freezeBaselineLocked(comment string) {
	if c.baselineFrozen {
		return
	}
	c.signature = BaselineSignature{
		Count:   c.cpuStats.Count(),
		CPU:     c.cpuStats.Summary(),
		Memory:  c.memStats.Summary(),
		Disk:    c.diskStats.Summary(),
		Comment: comment,
	}
	c.baselineFrozen = true
}

func (c *Collector) flagAnomalies(s Sample) Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flagAnomaliesLocked(s)
}

func (c *Collector) flagAnomaliesLocked(s Sample) Sample {
	reasons := detectAnomalies(s, c.signature, c.threshold)
	if len(reasons) > 0 {
		s.Anomaly = true
		s.Reasons = reasons
	}
	return s
}

// Report aggregates samples and metadata into a Report object.
func (c *Collector) Report(end time.Time, interval time.Duration, fault string) Report {
	if !c.baselineFrozen {
		c.freezeBaseline("baseline ended early")
	}

	c.mu.RLock()
	signature := c.signature
	start := c.startTime
	samples := c.samples
	c.mu.RUnlock()

	duration := end.Sub(c.startTime).Seconds()
	return Report{
		StartTime:     start,
		EndTime:       end,
		IntervalMs:    int(interval.Milliseconds()),
		DurationSec:   duration,
		Threshold:     c.threshold,
		FaultInjected: fault,
		Baseline:      signature,
		Samples:       samples,
	}
}

// BaselineSnapshot returns the current baseline summary and whether the baseline window is complete.
func (c *Collector) BaselineSnapshot() (BaselineSignature, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.baselineFrozen {
		return c.signature, true
	}
	sig := BaselineSignature{
		Count:   c.cpuStats.Count(),
		CPU:     c.cpuStats.Summary(),
		Memory:  c.memStats.Summary(),
		Disk:    c.diskStats.Summary(),
		Comment: "learning baseline",
	}
	return sig, c.baselineReady
}

func detectAnomalies(s Sample, baseline BaselineSignature, threshold float64) []string {
	var reasons []string
	check := func(value float64, sig MeanStd, label, unit string) {
		// If variance is tiny, fall back to a relative delta check.
		if sig.StdDev == 0 {
			if sig.Mean == 0 {
				return
			}
			if value > sig.Mean*1.25 {
				reasons = append(reasons, fmt.Sprintf("%s jumped to %.2f%s (flat baseline %.2f%s)", label, value, unit, sig.Mean, unit))
			}
			return
		}
		upper := sig.Mean + threshold*sig.StdDev
		if value > upper {
			reasons = append(reasons, fmt.Sprintf("%s high: %.2f%s (baseline %.2f ± %.2f%s)", label, value, unit, sig.Mean, sig.StdDev, unit))
		}
	}

	check(s.CPUPercent, baseline.CPU, "CPU", "%")
	check(s.MemPercent, baseline.Memory, "Memory", "%")
	check(s.DiskPercent, baseline.Disk, "Disk", "%")
	return reasons
}

// WriteJSON writes a report to disk for reuse by the HTML renderer.
func WriteJSON(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// FaultHandle allows stopping an injected slowdown.
type FaultHandle struct {
	cancel context.CancelFunc
	kind   string
}

// StartFault injects a synthetic slowdown fault (CPU busy loop or memory leak).
func StartFault(kind string, duration time.Duration, memTargetMB int) *FaultHandle {
	ctx, cancel := context.WithCancel(context.Background())
	handle := &FaultHandle{cancel: cancel, kind: kind}

	switch kind {
	case "cpu":
		startCPUHog(ctx)
	case "mem":
		if memTargetMB <= 0 {
			memTargetMB = 512
		}
		startMemoryPressure(ctx, memTargetMB)
	default:
		cancel()
		return nil
	}

	if duration > 0 {
		go func() {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			<-timer.C
			cancel()
		}()
	}
	return handle
}

func (f *FaultHandle) Stop() {
	if f != nil && f.cancel != nil {
		f.cancel()
	}
}

// CPU hog spins on all cores to inflate CPU usage.
func startCPUHog(ctx context.Context) {
	worker := func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = math.Sqrt(12345.6789) // prevent compiler from optimizing away the work
			}
		}
	}

	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		go worker()
	}
}

// Memory pressure repeatedly allocates until hitting the target footprint.
func startMemoryPressure(ctx context.Context, targetMB int) {
	const stepMB = 16
	targetBytes := targetMB * 1024 * 1024
	block := make([][]byte, 0)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(block)*stepMB*1024*1024 >= targetBytes {
				continue
			}
			chunk := make([]byte, stepMB*1024*1024)
			for i := range chunk {
				chunk[i] = byte(i)
			}
			block = append(block, chunk)
		}
	}
}
