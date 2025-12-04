package metrics

import (
	"runtime"
	"sync"
	"time"
)

// SystemMetrics represents a snapshot of system resource usage
type SystemMetrics struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUUsagePercent float64   `json:"cpu_usage_percent"`
	MemoryUsageMB   float64   `json:"memory_usage_mb"`
	MemoryAllocMB   float64   `json:"memory_alloc_mb"`
	GoroutineCount  int       `json:"goroutine_count"`
	GCPauseMs       float64   `json:"gc_pause_ms"`
	HeapObjectsCount uint64   `json:"heap_objects_count"`
}

// Collector collects system metrics periodically
type Collector struct {
	mu              sync.RWMutex
	metrics         []SystemMetrics
	maxSamples      int
	interval        time.Duration
	stopCh          chan struct{}
	lastCPUTime     time.Time
	lastNumGoroutine int
}

// NewCollector creates a new metrics collector
func NewCollector(interval time.Duration, maxSamples int) *Collector {
	return &Collector{
		metrics:    make([]SystemMetrics, 0, maxSamples),
		maxSamples: maxSamples,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start begins collecting metrics
func (c *Collector) Start() {
	go c.collectLoop()
}

// Stop stops the collector
func (c *Collector) Stop() {
	close(c.stopCh)
}

// GetMetrics returns all collected metrics
func (c *Collector) GetMetrics() []SystemMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]SystemMetrics, len(c.metrics))
	copy(result, c.metrics)
	return result
}

// GetLatestMetrics returns the most recent n metrics
func (c *Collector) GetLatestMetrics(n int) []SystemMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if n > len(c.metrics) {
		n = len(c.metrics)
	}

	result := make([]SystemMetrics, n)
	start := len(c.metrics) - n
	copy(result, c.metrics[start:])
	return result
}

// collectLoop runs the collection loop
func (c *Collector) collectLoop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Initial CPU time baseline
	c.lastCPUTime = time.Now()

	for {
		select {
		case <-ticker.C:
			metrics := c.collectMetrics()
			c.addMetrics(metrics)
		case <-c.stopCh:
			return
		}
	}
}

// collectMetrics gathers current system metrics
func (c *Collector) collectMetrics() SystemMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate CPU usage approximation based on goroutine activity
	numGoroutine := runtime.NumGoroutine()
	now := time.Now()
	elapsed := now.Sub(c.lastCPUTime).Seconds()

	// Approximate CPU usage based on goroutine changes and scheduling
	cpuUsage := 0.0
	if elapsed > 0 {
		goroutineDelta := float64(numGoroutine - c.lastNumGoroutine)
		// This is a heuristic approximation
		cpuUsage = (float64(numGoroutine) / float64(runtime.NumCPU())) * 10.0
		if cpuUsage > 100.0 {
			cpuUsage = 100.0
		}
		if goroutineDelta > 10 {
			cpuUsage += goroutineDelta * 0.5
			if cpuUsage > 100.0 {
				cpuUsage = 100.0
			}
		}
	}

	c.lastCPUTime = now
	c.lastNumGoroutine = numGoroutine

	// Convert to MB
	memUsageMB := float64(memStats.Sys) / (1024 * 1024)
	memAllocMB := float64(memStats.Alloc) / (1024 * 1024)

	// Get last GC pause time in milliseconds
	gcPause := 0.0
	if memStats.NumGC > 0 {
		gcPause = float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1e6
	}

	return SystemMetrics{
		Timestamp:        now,
		CPUUsagePercent:  cpuUsage,
		MemoryUsageMB:    memUsageMB,
		MemoryAllocMB:    memAllocMB,
		GoroutineCount:   numGoroutine,
		GCPauseMs:        gcPause,
		HeapObjectsCount: memStats.HeapObjects,
	}
}

// addMetrics adds a metric sample to the collection
func (c *Collector) addMetrics(m SystemMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add new metric
	c.metrics = append(c.metrics, m)

	// Remove oldest if over capacity
	if len(c.metrics) > c.maxSamples {
		c.metrics = c.metrics[1:]
	}
}

// Statistics computes statistical summary of collected metrics
type Statistics struct {
	CPUMean    float64 `json:"cpu_mean"`
	CPUStdDev  float64 `json:"cpu_stddev"`
	MemMean    float64 `json:"mem_mean"`
	MemStdDev  float64 `json:"mem_stddev"`
	GoroutineMean float64 `json:"goroutine_mean"`
	GoroutineStdDev float64 `json:"goroutine_stddev"`
}

// GetStatistics computes statistics over collected metrics
// Uses a sliding window of recent samples for dynamic baseline
func (c *Collector) GetStatistics() Statistics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.metrics) == 0 {
		return Statistics{}
	}

	// Use sliding window: last 30 samples for baseline (or all if less than 30)
	windowSize := 30
	samples := c.metrics
	if len(c.metrics) > windowSize {
		samples = c.metrics[len(c.metrics)-windowSize:]
	}

	var cpuSum, memSum, goroutineSum float64
	for _, m := range samples {
		cpuSum += m.CPUUsagePercent
		memSum += m.MemoryAllocMB
		goroutineSum += float64(m.GoroutineCount)
	}

	n := float64(len(samples))
	cpuMean := cpuSum / n
	memMean := memSum / n
	goroutineMean := goroutineSum / n

	// Calculate standard deviation
	var cpuVar, memVar, goroutineVar float64
	for _, m := range samples {
		cpuVar += (m.CPUUsagePercent - cpuMean) * (m.CPUUsagePercent - cpuMean)
		memVar += (m.MemoryAllocMB - memMean) * (m.MemoryAllocMB - memMean)
		gDiff := float64(m.GoroutineCount) - goroutineMean
		goroutineVar += gDiff * gDiff
	}

	return Statistics{
		CPUMean:         cpuMean,
		CPUStdDev:       sqrt(cpuVar / n),
		MemMean:         memMean,
		MemStdDev:       sqrt(memVar / n),
		GoroutineMean:   goroutineMean,
		GoroutineStdDev: sqrt(goroutineVar / n),
	}
}

// Simple square root approximation using Newton's method
func sqrt(x float64) float64 {
	if x == 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}
