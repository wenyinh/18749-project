package monitoring

import (
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

// FaultType represents the type of fault to inject
type FaultType string

const (
	FaultTypeCPU       FaultType = "cpu"        // CPU-intensive task
	FaultTypeMemory    FaultType = "memory"     // Memory leak
	FaultTypeGoroutine FaultType = "goroutine"  // Goroutine leak
	FaultTypeDiskIO    FaultType = "diskio"     // Disk I/O intensive
)

// FaultInjector allows injection of performance-slowdown faults
type FaultInjector struct {
	mu              sync.Mutex
	activeFaults    map[FaultType]bool
	stopChannels    map[FaultType]chan struct{}
	memoryLeakData  [][]byte
}

// NewFaultInjector creates a new fault injector
func NewFaultInjector() *FaultInjector {
	return &FaultInjector{
		activeFaults:   make(map[FaultType]bool),
		stopChannels:   make(map[FaultType]chan struct{}),
		memoryLeakData: make([][]byte, 0),
	}
}

// InjectFault starts injecting the specified fault
func (fi *FaultInjector) InjectFault(faultType FaultType, intensity int) error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if fi.activeFaults[faultType] {
		return fmt.Errorf("fault %s is already active", faultType)
	}

	stopCh := make(chan struct{})
	fi.activeFaults[faultType] = true
	fi.stopChannels[faultType] = stopCh

	log.Printf("[FAULT INJECTOR] 💉 Injecting %s fault with intensity %d", faultType, intensity)

	switch faultType {
	case FaultTypeCPU:
		go fi.injectCPUFault(stopCh, intensity)
	case FaultTypeMemory:
		go fi.injectMemoryLeak(stopCh, intensity)
	case FaultTypeGoroutine:
		go fi.injectGoroutineLeak(stopCh, intensity)
	case FaultTypeDiskIO:
		go fi.injectDiskIOFault(stopCh, intensity)
	default:
		delete(fi.activeFaults, faultType)
		delete(fi.stopChannels, faultType)
		return fmt.Errorf("unknown fault type: %s", faultType)
	}

	return nil
}

// StopFault stops the specified fault injection
func (fi *FaultInjector) StopFault(faultType FaultType) error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if !fi.activeFaults[faultType] {
		return fmt.Errorf("fault %s is not active", faultType)
	}

	log.Printf("[FAULT INJECTOR] 💊 Stopping %s fault", faultType)

	close(fi.stopChannels[faultType])
	delete(fi.activeFaults, faultType)
	delete(fi.stopChannels, faultType)

	// Clean up memory leak if applicable
	if faultType == FaultTypeMemory {
		fi.memoryLeakData = make([][]byte, 0)
		runtime.GC() // Force garbage collection
		log.Printf("[FAULT INJECTOR] Memory cleaned up, GC triggered")
	}

	return nil
}

// StopAllFaults stops all active fault injections
func (fi *FaultInjector) StopAllFaults() {
	fi.mu.Lock()
	faultTypes := make([]FaultType, 0, len(fi.activeFaults))
	for ft := range fi.activeFaults {
		faultTypes = append(faultTypes, ft)
	}
	fi.mu.Unlock()

	for _, ft := range faultTypes {
		_ = fi.StopFault(ft)
	}
}

// GetActiveFaults returns a list of currently active faults
func (fi *FaultInjector) GetActiveFaults() []FaultType {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	faults := make([]FaultType, 0, len(fi.activeFaults))
	for ft := range fi.activeFaults {
		faults = append(faults, ft)
	}
	return faults
}

// injectCPUFault creates CPU-intensive work
// Intensity: 1-10 (number of busy goroutines)
func (fi *FaultInjector) injectCPUFault(stopCh chan struct{}, intensity int) {
	if intensity <= 0 {
		intensity = 1
	}
	if intensity > 10 {
		intensity = 10
	}

	log.Printf("[FAULT INJECTOR] Starting CPU fault: %d busy goroutines", intensity)

	var wg sync.WaitGroup
	for i := 0; i < intensity; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			log.Printf("[FAULT INJECTOR] CPU worker %d started", id)

			for {
				select {
				case <-stopCh:
					log.Printf("[FAULT INJECTOR] CPU worker %d stopped", id)
					return
				default:
					// Busy work: compute something CPU-intensive
					_ = computePrimes(10000)
				}
			}
		}(i)
	}

	wg.Wait()
	log.Printf("[FAULT INJECTOR] All CPU workers stopped")
}

// computePrimes is a CPU-intensive operation
func computePrimes(limit int) []int {
	primes := []int{}
	for n := 2; n < limit; n++ {
		isPrime := true
		for p := 2; p*p <= n; p++ {
			if n%p == 0 {
				isPrime = false
				break
			}
		}
		if isPrime {
			primes = append(primes, n)
		}
	}
	return primes
}

// injectMemoryLeak allocates memory continuously without freeing it
// Intensity: MB per second to leak
func (fi *FaultInjector) injectMemoryLeak(stopCh chan struct{}, intensity int) {
	if intensity <= 0 {
		intensity = 10 // default 10 MB/s
	}

	log.Printf("[FAULT INJECTOR] Starting memory leak: %d MB/s", intensity)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			log.Printf("[FAULT INJECTOR] Memory leak stopped")
			return
		case <-ticker.C:
			// Allocate memory that won't be freed
			data := make([]byte, intensity*1024*1024) // intensity MB
			// Fill with random data to prevent optimization
			for i := range data {
				data[i] = byte(rand.Intn(256))
			}
			fi.mu.Lock()
			fi.memoryLeakData = append(fi.memoryLeakData, data)
			totalMB := len(fi.memoryLeakData) * intensity
			fi.mu.Unlock()
			log.Printf("[FAULT INJECTOR] Memory leaked: %d MB total", totalMB)
		}
	}
}

// injectGoroutineLeak creates goroutines that never exit
// Intensity: number of goroutines to create per second
func (fi *FaultInjector) injectGoroutineLeak(stopCh chan struct{}, intensity int) {
	if intensity <= 0 {
		intensity = 10 // default 10 goroutines/s
	}

	log.Printf("[FAULT INJECTOR] Starting goroutine leak: %d goroutines/s", intensity)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	goroutineCount := 0

	for {
		select {
		case <-stopCh:
			log.Printf("[FAULT INJECTOR] Goroutine leak stopped (created %d leaked goroutines)", goroutineCount)
			return
		case <-ticker.C:
			// Create goroutines that will leak (sleep forever or until stopCh closes)
			for i := 0; i < intensity; i++ {
				go func(id int) {
					// This goroutine will leak - it blocks forever or until stopped
					select {
					case <-stopCh:
						return
					case <-time.After(1 * time.Hour): // effectively forever
						return
					}
				}(goroutineCount)
				goroutineCount++
			}
			log.Printf("[FAULT INJECTOR] Goroutine leak: %d total goroutines created", goroutineCount)
		}
	}
}

// injectDiskIOFault creates disk I/O intensive operations
// Intensity: number of concurrent I/O operations
func (fi *FaultInjector) injectDiskIOFault(stopCh chan struct{}, intensity int) {
	if intensity <= 0 {
		intensity = 5
	}

	log.Printf("[FAULT INJECTOR] Starting disk I/O fault: %d concurrent operations", intensity)

	var wg sync.WaitGroup
	for i := 0; i < intensity; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			log.Printf("[FAULT INJECTOR] Disk I/O worker %d started", id)

			for {
				select {
				case <-stopCh:
					log.Printf("[FAULT INJECTOR] Disk I/O worker %d stopped", id)
					return
				default:
					// Simulate disk I/O by allocating and processing data
					data := make([]byte, 1024*1024) // 1 MB
					for i := range data {
						data[i] = byte(rand.Intn(256))
					}
					// Simulate processing delay
					time.Sleep(100 * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()
	log.Printf("[FAULT INJECTOR] All disk I/O workers stopped")
}

// GetFaultDescription returns a human-readable description of the fault
func GetFaultDescription(faultType FaultType) string {
	switch faultType {
	case FaultTypeCPU:
		return "CPU-intensive fault: Runs CPU-bound computations continuously"
	case FaultTypeMemory:
		return "Memory leak fault: Allocates memory without freeing it"
	case FaultTypeGoroutine:
		return "Goroutine leak fault: Creates goroutines that never exit"
	case FaultTypeDiskIO:
		return "Disk I/O fault: Performs intensive disk operations"
	default:
		return "Unknown fault type"
	}
}
