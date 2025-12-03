package monitor

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"time"
)

func startFaultInjection(ctx context.Context, opts CollectOptions) {
	fault := strings.ToLower(strings.TrimSpace(opts.FaultType))
	if fault == "" {
		return
	}
	switch fault {
	case "cpu":
		go runCPUHog(ctx, opts.FaultDelay, opts.FaultDuration)
	case "memory":
		go runMemoryPressure(ctx, opts.FaultDelay, opts.FaultDuration)
	default:
		log.Printf("[monitor] unsupported fault type '%s'", opts.FaultType)
	}
}

func describeFault(opts CollectOptions) string {
	parts := []string{strings.ToLower(strings.TrimSpace(opts.FaultType))}
	if opts.FaultDelay > 0 {
		parts = append(parts, fmt.Sprintf("delay=%s", opts.FaultDelay))
	}
	if opts.FaultDuration > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", opts.FaultDuration))
	}
	return strings.Join(parts, ", ")
}

func runCPUHog(ctx context.Context, delay, duration time.Duration) {
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
	log.Printf("[monitor] injecting CPU hog fault")

	hogCtx := ctx
	var cancel context.CancelFunc
	if duration > 0 {
		hogCtx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			seed := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			for {
				select {
				case <-hogCtx.Done():
					return
				default:
					// Tight math loop keeps CPU busy but deterministic.
					_ = math.Sin(float64(seed.Intn(1000))) * math.Cos(float64(seed.Intn(1000)))
				}
			}
		}(i)
	}

	<-hogCtx.Done()
	wg.Wait()
	log.Printf("[monitor] CPU hog fault finished")
}

func runMemoryPressure(ctx context.Context, delay, duration time.Duration) {
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
	log.Printf("[monitor] injecting memory pressure fault")

	hogCtx := ctx
	var cancel context.CancelFunc
	if duration > 0 {
		hogCtx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	const chunkSize = 10 * 1024 * 1024 // 10 MB per chunk
	const maxChunks = 30               // ~300 MB total

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	blobs := make([][]byte, 0, maxChunks)

	for {
		select {
		case <-hogCtx.Done():
			log.Printf("[monitor] memory fault finished")
			return
		case <-ticker.C:
			if len(blobs) >= maxChunks {
				continue
			}
			buf := make([]byte, chunkSize)
			for i := range buf {
				buf[i] = byte(i % 251)
			}
			blobs = append(blobs, buf)
		}
	}
}
