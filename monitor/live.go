package monitor

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// FaultStatus describes any currently running or recently finished fault injector.
type FaultStatus struct {
	Active                   bool      `json:"active"`
	Type                     string    `json:"type,omitempty"`
	DelaySeconds             float64   `json:"delay_seconds,omitempty"`
	RequestedDurationSeconds float64   `json:"requested_duration_seconds,omitempty"`
	StartedAt                time.Time `json:"started_at,omitempty"`
	CompletedAt              time.Time `json:"completed_at,omitempty"`
}

// LiveMonitor maintains a rolling history of samples for the UI mode.
type LiveMonitor struct {
	collector    *Collector
	historyLimit int
	sensitivity  float64

	mu       sync.RWMutex
	samples  []MetricSample
	metadata SnapshotMetadata
	baseline *BaselineSpec

	ctx context.Context

	faultCancel context.CancelFunc
	faultDone   <-chan struct{}
	faultInfo   FaultStatus
}

// NewLiveMonitor wires together the collector and rolling buffer for serve mode.
func NewLiveMonitor(interval time.Duration, diskPath string, historyLimit int, sensitivity float64) *LiveMonitor {
	if historyLimit <= 0 {
		historyLimit = 600
	}
	if sensitivity <= 0 {
		sensitivity = 3
	}
	collector := NewCollector(interval, diskPath)
	hostname, _ := os.Hostname()
	return &LiveMonitor{
		collector:    collector,
		historyLimit: historyLimit,
		sensitivity:  sensitivity,
		metadata: SnapshotMetadata{
			IntervalMillis: int(collector.interval / time.Millisecond),
			DiskPath:       collector.diskPath,
			Hostname:       hostname,
		},
	}
}

// Run starts the periodic sampling loop until the context is cancelled.
func (lm *LiveMonitor) Run(ctx context.Context) error {
	lm.mu.Lock()
	if lm.ctx != nil {
		lm.mu.Unlock()
		return fmt.Errorf("live monitor already running")
	}
	lm.ctx = ctx
	lm.mu.Unlock()

	if sample, err := lm.collector.sample(); err == nil {
		lm.addSample(sample)
	} else {
		log.Printf("[monitor] initial live sample failed: %v", err)
	}

	ticker := time.NewTicker(lm.collector.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			sample, err := lm.collector.sample()
			if err != nil {
				log.Printf("[monitor] live sample failed: %v", err)
				continue
			}
			lm.addSample(sample)
		}
	}
}

func (lm *LiveMonitor) addSample(sample MetricSample) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.samples = append(lm.samples, sample)
	if len(lm.samples) > lm.historyLimit {
		drop := len(lm.samples) - lm.historyLimit
		lm.samples = append([]MetricSample(nil), lm.samples[drop:]...)
	}
	if len(lm.samples) > 0 {
		lm.metadata.StartTime = lm.samples[0].Timestamp
		lm.metadata.EndTime = lm.samples[len(lm.samples)-1].Timestamp
		lm.metadata.DurationSeconds = lm.metadata.EndTime.Sub(lm.metadata.StartTime).Seconds()
		lm.metadata.SampleCount = len(lm.samples)
	}
}

// HistorySnapshot returns a copy of the current rolling window.
func (lm *LiveMonitor) HistorySnapshot() MetricsSnapshot {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	samples := make([]MetricSample, len(lm.samples))
	copy(samples, lm.samples)
	meta := lm.metadata
	meta.SampleCount = len(samples)
	return MetricsSnapshot{Metadata: meta, Samples: samples}
}

// Sensitivity returns the anomaly detection multiplier in use.
func (lm *LiveMonitor) Sensitivity() float64 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.sensitivity
}

// SetSensitivity updates the anomaly detection multiplier.
func (lm *LiveMonitor) SetSensitivity(v float64) {
	if v <= 0 {
		return
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.sensitivity = v
}

// Baseline returns the stored baseline if present.
func (lm *LiveMonitor) Baseline() (BaselineSpec, bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	if lm.baseline == nil {
		return BaselineSpec{}, false
	}
	copy := *lm.baseline
	return copy, true
}

// SetBaseline stores the provided baseline spec for anomaly detection.
func (lm *LiveMonitor) SetBaseline(b BaselineSpec) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	copy := b
	lm.baseline = &copy
}

// ClearBaseline removes any stored baseline signature.
func (lm *LiveMonitor) ClearBaseline() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.baseline = nil
}

// ComputeBaseline builds a baseline from the current samples.
func (lm *LiveMonitor) ComputeBaseline(minSamples int) (BaselineSpec, error) {
	if minSamples <= 0 {
		minSamples = 30
	}
	snap := lm.HistorySnapshot()
	if len(snap.Samples) < minSamples {
		return BaselineSpec{}, fmt.Errorf("need at least %d samples; have %d", minSamples, len(snap.Samples))
	}
	baseline := ComputeBaseline(snap)
	lm.SetBaseline(baseline)
	return baseline, nil
}

// FaultStatus returns information about the fault injector.
func (lm *LiveMonitor) FaultStatus() FaultStatus {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.faultInfo
}

// StartFault launches an artificial fault workload.
func (lm *LiveMonitor) StartFault(faultType string, delay, duration time.Duration) error {
	lm.mu.RLock()
	baseCtx := lm.ctx
	active := lm.faultCancel != nil
	lm.mu.RUnlock()

	if baseCtx == nil {
		return fmt.Errorf("live monitor is not running")
	}
	if active {
		return fmt.Errorf("a fault is already running")
	}

	cancel, done, err := LaunchFault(baseCtx, faultType, delay, duration)
	if err != nil {
		return err
	}

	lm.mu.Lock()
	lm.faultCancel = cancel
	lm.faultDone = done
	lm.faultInfo = FaultStatus{
		Active:                   true,
		Type:                     strings.ToLower(strings.TrimSpace(faultType)),
		DelaySeconds:             delay.Seconds(),
		RequestedDurationSeconds: duration.Seconds(),
		StartedAt:                time.Now(),
	}
	lm.faultInfo.CompletedAt = time.Time{}
	lm.mu.Unlock()

	go lm.watchFault(done)
	return nil
}

func (lm *LiveMonitor) watchFault(done <-chan struct{}) {
	<-done
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.faultCancel = nil
	lm.faultDone = nil
	lm.faultInfo.Active = false
	lm.faultInfo.CompletedAt = time.Now()
}

// StopFault cancels any running fault injector.
func (lm *LiveMonitor) StopFault() bool {
	lm.mu.RLock()
	cancel := lm.faultCancel
	done := lm.faultDone
	running := lm.faultInfo.Active
	lm.mu.RUnlock()

	if cancel == nil {
		return false
	}

	cancel()
	if done != nil {
		<-done
	}
	return running
}

// HistoryLimit returns the configured number of samples retained in memory.
func (lm *LiveMonitor) HistoryLimit() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.historyLimit
}
