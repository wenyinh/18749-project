```bash
# 1. Build
go build -o bin/server_metrics ./cmd/server_metrics

# 2. Start server (fast demo mode)
./bin/server_metrics \
  -id S1 \
  -port 9001 \
  -metrics-port 8080 \
  -max-samples 50

# 3. Open browser
# http://localhost:8080
```

### Demonstrate Memory Leak (3 minutes)

1. Click **"Inject Memory Leak"** button
2. Observe changes (wait 15-20 seconds):
   - **Memory graph**: Linear increase (+10MB/500ms)
   - **Memory anomaly card**: Turns red (CRITICAL)

3. Click **"Stop Fault Injection"** to stop

**Diagnosis key point**: Continuous linear memory growth = Memory leak

### Demonstrate CPU Intensive Fault (2 minutes)

1. Wait 30 seconds for metrics to return to normal
2. Click **"Inject CPU Spike"** button
3. Observe changes (wait 5-10 seconds):
   - **CPU graph**: Increases 2-3x (note: this is estimated value, not real CPU%)
   - **Goroutine graph**: Count increases (+N CPU cores)
   - **Goroutine anomaly card**: Turns red (CRITICAL)

### Demonstrate Goroutine Leak (3 minutes)

1. Wait 30 seconds for metrics to return to normal
2. Click **"Inject Goroutine Leak"** button
3. Observe changes (wait 30 seconds):
   - **Goroutine graph**: Continuous linear growth
   - **Memory graph**: Slow growth
   - **Goroutine anomaly card**: Turns red (CRITICAL)

**Notes**:
1. CPU metric is estimated (based on goroutines), not real CPU usage
2. Focus on **deviation from baseline** (whether exceeding 3σ), not absolute values
3. **Baseline uses sliding window** (last 30 samples), updates dynamically


## Fault Implementation Details

### 1. Memory Leak
```go
// Allocate 10MB memory every 500ms and keep reference
leak := make([]byte, 10*1024*1024)
leakedMemory = append(leakedMemory, leak)
```

### 2. CPU Intensive
```go
// Execute SHA-256 hashing on all CPU cores
for i := 0; i < runtime.NumCPU(); i++ {
    go cpuIntensiveWorker()
}
```

### 3. Goroutine Leak
```go
// Create goroutines that never terminate and block forever
blockCh := make(chan struct{})
go func() { <-blockCh }()
```

**Q: Port already in use?**
```bash
# Change metrics port
./bin/server_metrics -metrics-port 8081
```

```bash
# Fast mode
./bin/server_metrics -max-samples 50    # ~50 seconds

# Super fast mode
./bin/server_metrics -max-samples 30 -metrics-interval 500  # ~15 seconds
```
