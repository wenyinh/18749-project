package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

//go:embed monitor.html
var monitorPage []byte

type sample struct {
	Timestamp int64   `json:"ts"`
	CPU       float64 `json:"cpu"`
	MemMB     float64 `json:"mem_mb"`
	MemPct    float64 `json:"mem_pct"`
	Disk      float64 `json:"disk"`
}

type metricsServer struct {
	mu          sync.Mutex
	samples     []sample
	maxSamples  int
	faultCancel context.CancelFunc
	faultEnds   time.Time
	faultID     int64
}

type baseline struct {
	CPU   float64 `json:"cpu"`
	MemMB float64 `json:"mem_mb"`
	Disk  float64 `json:"disk"`
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address for the dashboard")
	interval := flag.Duration("interval", 1*time.Second, "sampling interval for metrics")
	faultDuration := flag.Duration("fault_duration", 10*time.Second, "duration of the injected CPU fault")
	flag.Parse()

	ms := &metricsServer{maxSamples: 360}
	ms.startCollector(*interval)

	mux := http.NewServeMux()
	mux.HandleFunc("/", servePage)
	mux.HandleFunc("/metrics", ms.metricsHandler)
	mux.HandleFunc("/fault", ms.handleFault(*faultDuration))

	log.Printf("[MON] starting dashboard on %s (interval=%s, fault=%s)", *addr, interval.String(), faultDuration.String())
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func servePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(monitorPage)
}

func (ms *metricsServer) startCollector(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()

		for range t.C {
			cpuUsage := readCPU()
			memUsedMB, memPct := readMem()
			diskUsage := readDisk()

			ms.mu.Lock()
			ms.samples = append(ms.samples, sample{
				Timestamp: time.Now().UnixMilli(),
				CPU:       cpuUsage,
				MemMB:     memUsedMB,
				MemPct:    memPct,
				Disk:      diskUsage,
			})
			if len(ms.samples) > ms.maxSamples {
				ms.samples = ms.samples[len(ms.samples)-ms.maxSamples:]
			}
			ms.mu.Unlock()
		}
	}()
}

func (ms *metricsServer) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ms.mu.Lock()
	resp := struct {
		Samples     []sample  `json:"samples"`
		Current     *sample   `json:"current,omitempty"`
		Baseline    *baseline `json:"baseline,omitempty"`
		FaultActive bool      `json:"fault_active"`
		FaultEnds   int64     `json:"fault_ends_at,omitempty"`
	}{
		Samples: append([]sample(nil), ms.samples...),
	}
	if n := len(ms.samples); n > 0 {
		last := ms.samples[n-1]
		resp.Current = &last
		resp.Baseline = computeBaselineFirstWindow(ms.samples, 10*time.Second)
	}
	if !ms.faultEnds.IsZero() && time.Now().Before(ms.faultEnds) {
		resp.FaultActive = true
		resp.FaultEnds = ms.faultEnds.UnixMilli()
	}
	ms.mu.Unlock()

	_ = json.NewEncoder(w).Encode(resp)
}

func (ms *metricsServer) handleFault(defaultDuration time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dur := defaultDuration
		if q := r.URL.Query().Get("duration"); q != "" {
			if parsed, err := time.ParseDuration(q); err == nil && parsed > 0 {
				dur = parsed
			}
		}

		ms.triggerCPUFault(dur)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"status":       "started",
			"duration_sec": fmt.Sprintf("%.1f", dur.Seconds()),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (ms *metricsServer) triggerCPUFault(dur time.Duration) {
	ms.mu.Lock()
	if ms.faultCancel != nil {
		ms.faultCancel()
	}
	ms.faultID++
	faultID := ms.faultID
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	ms.faultCancel = cancel
	ms.faultEnds = time.Now().Add(dur)
	ms.mu.Unlock()

	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					spin()
				}
			}
		}()
	}

	go func(id int64) {
		<-ctx.Done()
		wg.Wait()
		ms.mu.Lock()
		if ms.faultID == id {
			ms.faultCancel = nil
			ms.faultEnds = time.Time{}
		}
		ms.mu.Unlock()
		log.Printf("[MON] CPU fault finished")
	}(faultID)

	log.Printf("[MON] CPU fault injected for %s using %d workers", dur, workers)
}

func spin() {
	// Simple busy loop to consume CPU cycles
	for i := 0; i < 1_000_000; i++ {
		_ = i * i
	}
}

func readCPU() float64 {
	usage, err := cpu.Percent(0, false)
	if err != nil {
		log.Printf("[MON] CPU read error: %v", err)
		return 0
	}
	if len(usage) == 0 {
		return 0
	}
	return usage[0]
}

func readMem() (float64, float64) {
	memStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("[MON] mem read error: %v", err)
		return 0, 0
	}
	usedMB := float64(memStat.Used) / (1024 * 1024)
	return usedMB, memStat.UsedPercent
}

func readDisk() float64 {
	info, err := disk.Usage("/")
	if err != nil {
		log.Printf("[MON] disk read error: %v", err)
		return 0
	}
	return info.UsedPercent
}

func computeBaselineFirstWindow(samples []sample, window time.Duration) *baseline {
	if len(samples) == 0 {
		return nil
	}
	startTS := samples[0].Timestamp
	cutoff := startTS + window.Milliseconds()
	var totalCPU, totalMemMB, totalDisk float64
	var count int
	for _, s := range samples {
		if s.Timestamp <= cutoff {
			totalCPU += s.CPU
			totalMemMB += s.MemMB
			totalDisk += s.Disk
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return &baseline{
		CPU:   totalCPU / float64(count),
		MemMB: totalMemMB / float64(count),
		Disk:  totalDisk / float64(count),
	}
}
