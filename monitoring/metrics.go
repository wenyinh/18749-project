package monitoring

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// SystemMetrics holds black-box OS-level metrics
type SystemMetrics struct {
	Timestamp       time.Time `json:"timestamp"`
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryUsedMB    uint64    `json:"memory_used_mb"`
	MemoryPercent   float64   `json:"memory_percent"`
	DiskReadMB      uint64    `json:"disk_read_mb"`
	DiskWriteMB     uint64    `json:"disk_write_mb"`
	NetworkSentMB   uint64    `json:"network_sent_mb"`
	NetworkRecvMB   uint64    `json:"network_recv_mb"`
	GoroutineCount  int       `json:"goroutine_count"`
	HeapAllocMB     uint64    `json:"heap_alloc_mb"`
	ProcessCPU      float64   `json:"process_cpu"`
	ProcessMemoryMB uint64    `json:"process_memory_mb"`
}

// MetricsCollector periodically collects system metrics
type MetricsCollector struct {
	mu              sync.RWMutex
	metrics         []SystemMetrics
	maxHistory      int
	interval        time.Duration
	stopCh          chan struct{}
	componentID     string
	lastDiskIO      *disk.IOCountersStat
	lastNetIO       *net.IOCountersStat
	currentPID      int32
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(componentID string, interval time.Duration, maxHistory int) *MetricsCollector {
	if maxHistory <= 0 {
		maxHistory = 1000 // default to keeping last 1000 samples
	}
	return &MetricsCollector{
		metrics:     make([]SystemMetrics, 0, maxHistory),
		maxHistory:  maxHistory,
		interval:    interval,
		stopCh:      make(chan struct{}),
		componentID: componentID,
		currentPID:  int32(0), // will be set when process is created
	}
}

// Start begins collecting metrics at the specified interval
func (mc *MetricsCollector) Start() {
	go mc.collectLoop()
}

// Stop stops the metrics collection
func (mc *MetricsCollector) Stop() {
	close(mc.stopCh)
}

// SetProcessPID sets the PID of the process to monitor
func (mc *MetricsCollector) SetProcessPID(pid int32) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.currentPID = pid
}

func (mc *MetricsCollector) collectLoop() {
	ticker := time.NewTicker(mc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-mc.stopCh:
			return
		case <-ticker.C:
			metrics := mc.collectMetrics()
			mc.mu.Lock()
			mc.metrics = append(mc.metrics, metrics)
			if len(mc.metrics) > mc.maxHistory {
				mc.metrics = mc.metrics[1:]
			}
			mc.mu.Unlock()
		}
	}
}

func (mc *MetricsCollector) collectMetrics() SystemMetrics {
	now := time.Now()

	// CPU usage (system-wide)
	cpuPercent, _ := cpu.Percent(0, false)
	var cpuUsage float64
	if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	// Memory usage
	vmem, _ := mem.VirtualMemory()
	var memUsedMB uint64
	var memPercent float64
	if vmem != nil {
		memUsedMB = vmem.Used / 1024 / 1024
		memPercent = vmem.UsedPercent
	}

	// Disk I/O
	diskIO, _ := disk.IOCounters()
	var diskReadMB, diskWriteMB uint64
	if len(diskIO) > 0 {
		// Sum across all disks
		var totalRead, totalWrite uint64
		for _, io := range diskIO {
			totalRead += io.ReadBytes
			totalWrite += io.WriteBytes
		}
		diskReadMB = totalRead / 1024 / 1024
		diskWriteMB = totalWrite / 1024 / 1024
	}

	// Network I/O
	netIO, _ := net.IOCounters(false)
	var netSentMB, netRecvMB uint64
	if len(netIO) > 0 {
		netSentMB = netIO[0].BytesSent / 1024 / 1024
		netRecvMB = netIO[0].BytesRecv / 1024 / 1024
	}

	// Go runtime metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	heapAllocMB := m.Alloc / 1024 / 1024
	goroutines := runtime.NumGoroutine()

	// Process-specific metrics
	var processCPU float64
	var processMemMB uint64
	mc.mu.RLock()
	pid := mc.currentPID
	mc.mu.RUnlock()

	if pid > 0 {
		proc, err := process.NewProcess(pid)
		if err == nil {
			processCPU, _ = proc.CPUPercent()
			memInfo, err := proc.MemoryInfo()
			if err == nil {
				processMemMB = memInfo.RSS / 1024 / 1024
			}
		}
	}

	return SystemMetrics{
		Timestamp:       now,
		CPUPercent:      cpuUsage,
		MemoryUsedMB:    memUsedMB,
		MemoryPercent:   memPercent,
		DiskReadMB:      diskReadMB,
		DiskWriteMB:     diskWriteMB,
		NetworkSentMB:   netSentMB,
		NetworkRecvMB:   netRecvMB,
		GoroutineCount:  goroutines,
		HeapAllocMB:     heapAllocMB,
		ProcessCPU:      processCPU,
		ProcessMemoryMB: processMemMB,
	}
}

// GetLatestMetrics returns the most recent metrics
func (mc *MetricsCollector) GetLatestMetrics() *SystemMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if len(mc.metrics) == 0 {
		return nil
	}
	latest := mc.metrics[len(mc.metrics)-1]
	return &latest
}

// GetAllMetrics returns all collected metrics
func (mc *MetricsCollector) GetAllMetrics() []SystemMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	result := make([]SystemMetrics, len(mc.metrics))
	copy(result, mc.metrics)
	return result
}

// GetMetricsSince returns metrics since the given time
func (mc *MetricsCollector) GetMetricsSince(since time.Time) []SystemMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var result []SystemMetrics
	for _, m := range mc.metrics {
		if m.Timestamp.After(since) {
			result = append(result, m)
		}
	}
	return result
}

// PrintLatest prints the latest metrics to stdout
func (mc *MetricsCollector) PrintLatest() {
	latest := mc.GetLatestMetrics()
	if latest == nil {
		fmt.Printf("[%s] No metrics collected yet\n", mc.componentID)
		return
	}

	fmt.Printf("\n=== [%s] Black-Box Metrics ===\n", mc.componentID)
	fmt.Printf("Time: %s\n", latest.Timestamp.Format("15:04:05"))
	fmt.Printf("CPU: %.2f%%\n", latest.CPUPercent)
	fmt.Printf("Memory: %d MB (%.2f%%)\n", latest.MemoryUsedMB, latest.MemoryPercent)
	fmt.Printf("Disk: Read=%d MB, Write=%d MB\n", latest.DiskReadMB, latest.DiskWriteMB)
	fmt.Printf("Network: Sent=%d MB, Recv=%d MB\n", latest.NetworkSentMB, latest.NetworkRecvMB)
	fmt.Printf("Goroutines: %d\n", latest.GoroutineCount)
	fmt.Printf("Heap: %d MB\n", latest.HeapAllocMB)
	if latest.ProcessCPU > 0 {
		fmt.Printf("Process CPU: %.2f%%\n", latest.ProcessCPU)
		fmt.Printf("Process Memory: %d MB\n", latest.ProcessMemoryMB)
	}
	fmt.Printf("==============================\n\n")
}
