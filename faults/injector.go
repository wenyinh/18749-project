package faults

import (
	"crypto/sha256"
	"log"
	"runtime"
	"sync"
	"time"
)

// FaultType represents different types of faults that can be injected
type FaultType int

const (
	NoFault FaultType = iota
	MemoryLeak
	CPUIntensive
	GoroutineLeak
)

// Injector manages fault injection
type Injector struct {
	mu            sync.RWMutex
	activeFault   FaultType
	stopCh        chan struct{}
	leakedMemory  [][]byte // For memory leak simulation
	cpuWorkers    int
}

// NewInjector creates a new fault injector
func NewInjector() *Injector {
	return &Injector{
		activeFault:  NoFault,
		leakedMemory: make([][]byte, 0),
		cpuWorkers:   0,
	}
}

// InjectFault starts injecting a specific fault
func (f *Injector) InjectFault(faultType FaultType) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Stop any existing fault first
	f.stopCurrentFault()

	f.activeFault = faultType
	f.stopCh = make(chan struct{})

	switch faultType {
	case MemoryLeak:
		log.Printf("[FAULT] Injecting memory leak fault")
		go f.memoryLeakWorker()
	case CPUIntensive:
		log.Printf("[FAULT] Injecting CPU-intensive fault")
		// Start multiple CPU-intensive workers
		numWorkers := runtime.NumCPU()
		f.cpuWorkers = numWorkers
		for i := 0; i < numWorkers; i++ {
			go f.cpuIntensiveWorker(i)
		}
	case GoroutineLeak:
		log.Printf("[FAULT] Injecting goroutine leak fault")
		go f.goroutineLeakWorker()
	case NoFault:
		log.Printf("[FAULT] Clearing all faults")
	}
}

// StopFault stops the current fault injection
func (f *Injector) StopFault() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCurrentFault()
	f.activeFault = NoFault
}

// stopCurrentFault stops the currently running fault (must hold lock)
func (f *Injector) stopCurrentFault() {
	if f.stopCh != nil {
		close(f.stopCh)
		f.stopCh = nil
	}

	// Clean up leaked memory
	if len(f.leakedMemory) > 0 {
		f.leakedMemory = make([][]byte, 0)
		runtime.GC() // Force garbage collection
		log.Printf("[FAULT] Cleaned up leaked memory, forced GC")
	}
}

// GetActiveFault returns the currently active fault
func (f *Injector) GetActiveFault() FaultType {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.activeFault
}

// memoryLeakWorker simulates a memory leak by allocating memory periodically
func (f *Injector) memoryLeakWorker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	allocSize := 10 * 1024 * 1024 // 10MB per allocation

	for {
		select {
		case <-ticker.C:
			// Allocate memory and keep reference to prevent GC
			leak := make([]byte, allocSize)
			// Fill with some data to ensure allocation
			for i := 0; i < len(leak); i += 4096 {
				leak[i] = byte(i % 256)
			}

			f.mu.Lock()
			f.leakedMemory = append(f.leakedMemory, leak)
			totalLeaked := len(f.leakedMemory) * allocSize / (1024 * 1024)
			f.mu.Unlock()

			log.Printf("[FAULT] Memory leak: allocated %d MB total", totalLeaked)

		case <-f.stopCh:
			log.Printf("[FAULT] Memory leak worker stopped")
			return
		}
	}
}

// cpuIntensiveWorker performs CPU-intensive operations
func (f *Injector) cpuIntensiveWorker(id int) {
	log.Printf("[FAULT] CPU worker %d started", id)

	for {
		// Do work in smaller batches and check stop signal more frequently
		for batch := 0; batch < 100; batch++ {
			data := make([]byte, 1024)
			for i := 0; i < 1000; i++ {
				hash := sha256.Sum256(data)
				data = hash[:]
			}
		}

		// Check stop signal after batch
		select {
		case <-f.stopCh:
			log.Printf("[FAULT] CPU worker %d stopped", id)
			return
		default:
			// Continue next batch
		}
	}
}

// goroutineLeakWorker creates goroutines that never terminate
func (f *Injector) goroutineLeakWorker() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Create a goroutine that blocks indefinitely
			blockCh := make(chan struct{})
			go func() {
				<-blockCh // This will never receive, goroutine leaks
			}()

			// Also create some goroutines doing busy work
			for i := 0; i < 10; i++ {
				go func() {
					// Do some work then wait
					time.Sleep(10 * time.Millisecond)
					<-f.stopCh
				}()
			}

			currentGoroutines := runtime.NumGoroutine()
			log.Printf("[FAULT] Goroutine leak: current count = %d", currentGoroutines)

		case <-f.stopCh:
			log.Printf("[FAULT] Goroutine leak worker stopped (leaked goroutines will remain)")
			return
		}
	}
}

// String returns the string representation of FaultType
func (ft FaultType) String() string {
	switch ft {
	case NoFault:
		return "NoFault"
	case MemoryLeak:
		return "MemoryLeak"
	case CPUIntensive:
		return "CPUIntensive"
	case GoroutineLeak:
		return "GoroutineLeak"
	default:
		return "Unknown"
	}
}
