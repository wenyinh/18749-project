package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/lfd"
)

// go run cmd/lfd/lrunner.go -target 127.0.0.1:9001 -gfd 172.26.12.248:8000 -id LFD1 -server-id S1 -server-addr 0.0.0.0:9001 -rm 172.26.12.248:8001 -backups "S2=172.26.42.104:9002,S3=172.26.113.110:9003"
func main() {
	// Original parameters
	targetAddr := flag.String("target", "127.0.0.1:9000", "server address to monitor")
	hb := flag.Duration("hb", 1*time.Second, "heartbeat frequency (e.g. 1s, 500ms)")
	timeout := flag.Duration("timeout", 3*time.Second, "heartbeat timeout (e.g. 3s)")
	lfdID := flag.String("id", "LFD1", "LFD identifier")
	gfdAddr := flag.String("gfd", "127.0.0.1:8000", "GFD address")
	maxRetries := flag.Int("max-retries", 3, "maximum reconnection attempts")
	baseDelay := flag.Duration("base-delay", 1*time.Second, "base delay for exponential backoff")
	maxDelay := flag.Duration("max-delay", 10*time.Second, "maximum delay for exponential backoff")

	// New parameters for auto-recovery
	serverID := flag.String("server-id", "", "Server ID (e.g., S1)")
	serverAddr := flag.String("server-addr", "", "Server listen address (e.g., 0.0.0.0:9001)")
	rmAddr := flag.String("rm", "", "RM address (e.g., 127.0.0.1:8001)")
	backups := flag.String("backups", "", "Other servers (e.g., S2=127.0.0.1:9002,S3=127.0.0.1:9003)")
	ckptMs := flag.Int("ckpt-ms", 5000, "Checkpoint interval in milliseconds")

	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// Create LFD config
	config := lfd.LFDConfig{
		LFDID:      *lfdID,
		TargetAddr: *targetAddr,
		GFDAddr:    *gfdAddr,
		HBFreq:     *hb,
		Timeout:    *timeout,
		MaxRetries: *maxRetries,
		BaseDelay:  *baseDelay,
		MaxDelay:   *maxDelay,

		// Auto-recovery config
		ServerID:   *serverID,
		ServerAddr: *serverAddr,
		RMAddr:     *rmAddr,
		Backups:    *backups,
		CkptMs:     *ckptMs,
	}

	l := lfd.NewLFDWithConfig(config)
	if err := l.Run(); err != nil {
		log.Fatal(err)
	}
}
