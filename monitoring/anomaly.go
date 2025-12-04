package monitoring

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Baseline represents the normal behavior signature
type Baseline struct {
	CPUMean          float64 `json:"cpu_mean"`
	CPUStdDev        float64 `json:"cpu_stddev"`
	MemoryMean       float64 `json:"memory_mean"`
	MemoryStdDev     float64 `json:"memory_stddev"`
	GoroutinesMean   float64 `json:"goroutines_mean"`
	GoroutinesStdDev float64 `json:"goroutines_stddev"`
	HeapMean         float64 `json:"heap_mean"`
	HeapStdDev       float64 `json:"heap_stddev"`
	ProcessCPUMean   float64 `json:"process_cpu_mean"`
	ProcessCPUStdDev float64 `json:"process_cpu_stddev"`
	SampleCount      int     `json:"sample_count"`
	CreatedAt        time.Time `json:"created_at"`
}

// Anomaly represents a detected anomaly
type Anomaly struct {
	Timestamp    time.Time   `json:"timestamp"`
	Type         string      `json:"type"`         // "cpu", "memory", "goroutines", "heap"
	Severity     string      `json:"severity"`     // "low", "medium", "high", "critical"
	Value        float64     `json:"value"`
	Baseline     float64     `json:"baseline"`
	Deviation    float64     `json:"deviation"`    // in standard deviations
	Description  string      `json:"description"`
}

// AnomalyDetector detects anomalies based on baseline
type AnomalyDetector struct {
	mu              sync.RWMutex
	baseline        *Baseline
	anomalies       []Anomaly
	maxAnomalies    int
	thresholdSigma  float64  // number of standard deviations for anomaly
	criticalSigma   float64  // number of standard deviations for critical anomaly
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector(thresholdSigma, criticalSigma float64, maxAnomalies int) *AnomalyDetector {
	if maxAnomalies <= 0 {
		maxAnomalies = 100
	}
	if thresholdSigma <= 0 {
		thresholdSigma = 2.0 // 2 standard deviations
	}
	if criticalSigma <= 0 {
		criticalSigma = 4.0 // 4 standard deviations
	}
	return &AnomalyDetector{
		anomalies:      make([]Anomaly, 0, maxAnomalies),
		maxAnomalies:   maxAnomalies,
		thresholdSigma: thresholdSigma,
		criticalSigma:  criticalSigma,
	}
}

// EstablishBaseline calculates the baseline from historical metrics
func (ad *AnomalyDetector) EstablishBaseline(metrics []SystemMetrics) error {
	if len(metrics) < 10 {
		return fmt.Errorf("need at least 10 samples to establish baseline, got %d", len(metrics))
	}

	// Calculate means
	var cpuSum, memSum, goroutineSum, heapSum, processCPUSum float64
	for _, m := range metrics {
		cpuSum += m.CPUPercent
		memSum += float64(m.MemoryUsedMB)
		goroutineSum += float64(m.GoroutineCount)
		heapSum += float64(m.HeapAllocMB)
		processCPUSum += m.ProcessCPU
	}

	n := float64(len(metrics))
	cpuMean := cpuSum / n
	memMean := memSum / n
	goroutineMean := goroutineSum / n
	heapMean := heapSum / n
	processCPUMean := processCPUSum / n

	// Calculate standard deviations
	var cpuVariance, memVariance, goroutineVariance, heapVariance, processCPUVariance float64
	for _, m := range metrics {
		cpuVariance += math.Pow(m.CPUPercent-cpuMean, 2)
		memVariance += math.Pow(float64(m.MemoryUsedMB)-memMean, 2)
		goroutineVariance += math.Pow(float64(m.GoroutineCount)-goroutineMean, 2)
		heapVariance += math.Pow(float64(m.HeapAllocMB)-heapMean, 2)
		processCPUVariance += math.Pow(m.ProcessCPU-processCPUMean, 2)
	}

	cpuStdDev := math.Sqrt(cpuVariance / n)
	memStdDev := math.Sqrt(memVariance / n)
	goroutineStdDev := math.Sqrt(goroutineVariance / n)
	heapStdDev := math.Sqrt(heapVariance / n)
	processCPUStdDev := math.Sqrt(processCPUVariance / n)

	baseline := &Baseline{
		CPUMean:          cpuMean,
		CPUStdDev:        cpuStdDev,
		MemoryMean:       memMean,
		MemoryStdDev:     memStdDev,
		GoroutinesMean:   goroutineMean,
		GoroutinesStdDev: goroutineStdDev,
		HeapMean:         heapMean,
		HeapStdDev:       heapStdDev,
		ProcessCPUMean:   processCPUMean,
		ProcessCPUStdDev: processCPUStdDev,
		SampleCount:      len(metrics),
		CreatedAt:        time.Now(),
	}

	ad.mu.Lock()
	ad.baseline = baseline
	ad.mu.Unlock()

	return nil
}

// GetBaseline returns the current baseline
func (ad *AnomalyDetector) GetBaseline() *Baseline {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	return ad.baseline
}

// CheckAnomaly checks if the given metrics deviate from baseline
func (ad *AnomalyDetector) CheckAnomaly(metrics SystemMetrics) []Anomaly {
	ad.mu.RLock()
	baseline := ad.baseline
	ad.mu.RUnlock()

	if baseline == nil {
		return nil
	}

	var anomalies []Anomaly

	// Check CPU anomaly
	if baseline.CPUStdDev > 0 {
		cpuDeviation := (metrics.CPUPercent - baseline.CPUMean) / baseline.CPUStdDev
		if math.Abs(cpuDeviation) > ad.thresholdSigma {
			severity := ad.calculateSeverity(cpuDeviation)
			anomalies = append(anomalies, Anomaly{
				Timestamp:   metrics.Timestamp,
				Type:        "cpu",
				Severity:    severity,
				Value:       metrics.CPUPercent,
				Baseline:    baseline.CPUMean,
				Deviation:   cpuDeviation,
				Description: fmt.Sprintf("CPU usage %.2f%% deviates from baseline %.2f%% by %.2f standard deviations",
					metrics.CPUPercent, baseline.CPUMean, cpuDeviation),
			})
		}
	}

	// Check Memory anomaly
	if baseline.MemoryStdDev > 0 {
		memDeviation := (float64(metrics.MemoryUsedMB) - baseline.MemoryMean) / baseline.MemoryStdDev
		if math.Abs(memDeviation) > ad.thresholdSigma {
			severity := ad.calculateSeverity(memDeviation)
			anomalies = append(anomalies, Anomaly{
				Timestamp:   metrics.Timestamp,
				Type:        "memory",
				Severity:    severity,
				Value:       float64(metrics.MemoryUsedMB),
				Baseline:    baseline.MemoryMean,
				Deviation:   memDeviation,
				Description: fmt.Sprintf("Memory usage %d MB deviates from baseline %.2f MB by %.2f standard deviations",
					metrics.MemoryUsedMB, baseline.MemoryMean, memDeviation),
			})
		}
	}

	// Check Goroutines anomaly
	if baseline.GoroutinesStdDev > 0 {
		goroutineDeviation := (float64(metrics.GoroutineCount) - baseline.GoroutinesMean) / baseline.GoroutinesStdDev
		if math.Abs(goroutineDeviation) > ad.thresholdSigma {
			severity := ad.calculateSeverity(goroutineDeviation)
			anomalies = append(anomalies, Anomaly{
				Timestamp:   metrics.Timestamp,
				Type:        "goroutines",
				Severity:    severity,
				Value:       float64(metrics.GoroutineCount),
				Baseline:    baseline.GoroutinesMean,
				Deviation:   goroutineDeviation,
				Description: fmt.Sprintf("Goroutine count %d deviates from baseline %.2f by %.2f standard deviations",
					metrics.GoroutineCount, baseline.GoroutinesMean, goroutineDeviation),
			})
		}
	}

	// Check Heap anomaly
	if baseline.HeapStdDev > 0 {
		heapDeviation := (float64(metrics.HeapAllocMB) - baseline.HeapMean) / baseline.HeapStdDev
		if math.Abs(heapDeviation) > ad.thresholdSigma {
			severity := ad.calculateSeverity(heapDeviation)
			anomalies = append(anomalies, Anomaly{
				Timestamp:   metrics.Timestamp,
				Type:        "heap",
				Severity:    severity,
				Value:       float64(metrics.HeapAllocMB),
				Baseline:    baseline.HeapMean,
				Deviation:   heapDeviation,
				Description: fmt.Sprintf("Heap allocation %d MB deviates from baseline %.2f MB by %.2f standard deviations",
					metrics.HeapAllocMB, baseline.HeapMean, heapDeviation),
			})
		}
	}

	// Check Process CPU anomaly
	if baseline.ProcessCPUStdDev > 0 && metrics.ProcessCPU > 0 {
		processCPUDeviation := (metrics.ProcessCPU - baseline.ProcessCPUMean) / baseline.ProcessCPUStdDev
		if math.Abs(processCPUDeviation) > ad.thresholdSigma {
			severity := ad.calculateSeverity(processCPUDeviation)
			anomalies = append(anomalies, Anomaly{
				Timestamp:   metrics.Timestamp,
				Type:        "process_cpu",
				Severity:    severity,
				Value:       metrics.ProcessCPU,
				Baseline:    baseline.ProcessCPUMean,
				Deviation:   processCPUDeviation,
				Description: fmt.Sprintf("Process CPU usage %.2f%% deviates from baseline %.2f%% by %.2f standard deviations",
					metrics.ProcessCPU, baseline.ProcessCPUMean, processCPUDeviation),
			})
		}
	}

	// Store anomalies
	if len(anomalies) > 0 {
		ad.mu.Lock()
		ad.anomalies = append(ad.anomalies, anomalies...)
		if len(ad.anomalies) > ad.maxAnomalies {
			ad.anomalies = ad.anomalies[len(ad.anomalies)-ad.maxAnomalies:]
		}
		ad.mu.Unlock()
	}

	return anomalies
}

func (ad *AnomalyDetector) calculateSeverity(deviation float64) string {
	absDeviation := math.Abs(deviation)
	if absDeviation >= ad.criticalSigma {
		return "critical"
	} else if absDeviation >= ad.thresholdSigma+1.5 {
		return "high"
	} else if absDeviation >= ad.thresholdSigma+0.5 {
		return "medium"
	}
	return "low"
}

// GetAnomalies returns all detected anomalies
func (ad *AnomalyDetector) GetAnomalies() []Anomaly {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	result := make([]Anomaly, len(ad.anomalies))
	copy(result, ad.anomalies)
	return result
}

// GetRecentAnomalies returns anomalies from the last duration
func (ad *AnomalyDetector) GetRecentAnomalies(duration time.Duration) []Anomaly {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	cutoff := time.Now().Add(-duration)
	var result []Anomaly
	for _, a := range ad.anomalies {
		if a.Timestamp.After(cutoff) {
			result = append(result, a)
		}
	}
	return result
}

// DiagnoseRootCause provides diagnosis based on detected anomalies
func (ad *AnomalyDetector) DiagnoseRootCause(anomalies []Anomaly) string {
	if len(anomalies) == 0 {
		return "No anomalies detected - system operating normally"
	}

	// Count anomaly types
	typeCounts := make(map[string]int)
	severityCounts := make(map[string]int)
	for _, a := range anomalies {
		typeCounts[a.Type]++
		severityCounts[a.Severity]++
	}

	diagnosis := "\n=== ROOT CAUSE ANALYSIS ===\n"

	// Check for CPU-intensive fault
	if typeCounts["cpu"] > 0 || typeCounts["process_cpu"] > 0 {
		diagnosis += "\n🔴 CPU PERFORMANCE FAULT DETECTED\n"
		diagnosis += fmt.Sprintf("   - CPU anomalies: %d occurrences\n", typeCounts["cpu"])
		diagnosis += fmt.Sprintf("   - Process CPU anomalies: %d occurrences\n", typeCounts["process_cpu"])
		diagnosis += "   Diagnosis: Likely CPU-intensive task or infinite loop\n"
		diagnosis += "   Recommendation: Check for CPU-bound operations, inefficient algorithms\n"
	}

	// Check for memory leak
	if typeCounts["memory"] > 0 || typeCounts["heap"] > 0 {
		diagnosis += "\n🔴 MEMORY FAULT DETECTED\n"
		diagnosis += fmt.Sprintf("   - Memory anomalies: %d occurrences\n", typeCounts["memory"])
		diagnosis += fmt.Sprintf("   - Heap anomalies: %d occurrences\n", typeCounts["heap"])
		diagnosis += "   Diagnosis: Possible memory leak or excessive allocation\n"
		diagnosis += "   Recommendation: Check for memory leaks, unused object retention\n"
	}

	// Check for goroutine leak
	if typeCounts["goroutines"] > 0 {
		diagnosis += "\n🔴 GOROUTINE LEAK DETECTED\n"
		diagnosis += fmt.Sprintf("   - Goroutine anomalies: %d occurrences\n", typeCounts["goroutines"])
		diagnosis += "   Diagnosis: Goroutine leak - threads not being cleaned up\n"
		diagnosis += "   Recommendation: Check for goroutines that never exit, missing done channels\n"
	}

	// Severity summary
	diagnosis += "\n=== SEVERITY SUMMARY ===\n"
	for severity, count := range severityCounts {
		diagnosis += fmt.Sprintf("   %s: %d\n", severity, count)
	}

	diagnosis += "========================\n\n"

	return diagnosis
}

// PrintBaseline prints the baseline statistics
func (ad *AnomalyDetector) PrintBaseline() {
	baseline := ad.GetBaseline()
	if baseline == nil {
		fmt.Println("No baseline established yet")
		return
	}

	fmt.Println("\n=== NORMAL BEHAVIOR BASELINE ===")
	fmt.Printf("Established at: %s\n", baseline.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Sample count: %d\n\n", baseline.SampleCount)
	fmt.Printf("CPU:        %.2f%% (σ=%.2f)\n", baseline.CPUMean, baseline.CPUStdDev)
	fmt.Printf("Memory:     %.2f MB (σ=%.2f)\n", baseline.MemoryMean, baseline.MemoryStdDev)
	fmt.Printf("Goroutines: %.2f (σ=%.2f)\n", baseline.GoroutinesMean, baseline.GoroutinesStdDev)
	fmt.Printf("Heap:       %.2f MB (σ=%.2f)\n", baseline.HeapMean, baseline.HeapStdDev)
	if baseline.ProcessCPUMean > 0 {
		fmt.Printf("Process CPU: %.2f%% (σ=%.2f)\n", baseline.ProcessCPUMean, baseline.ProcessCPUStdDev)
	}
	fmt.Println("================================\n")
}
