package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/lfd"
)

// go run lrunner.go -target 127.0.0.1:9001 -gfd 127.0.0.1:8000 -id LFD1
func main() {
	targetAddr := flag.String("target", "127.0.0.1:9000", "server address to monitor")
	hb := flag.Duration("hb", 1*time.Second, "heartbeat frequency (e.g. 1s, 500ms)")
	timeout := flag.Duration("timeout", 3*time.Second, "heartbeat timeout (e.g. 3s)")
	lfdID := flag.String("id", "LFD1", "LFD identifier")
	gfdAddr := flag.String("gfd", "127.0.0.1:8000", "GFD address")
	maxRetries := flag.Int("max-retries", 3, "maximum reconnection attempts")
	baseDelay := flag.Duration("base-delay", 1*time.Second, "base delay for exponential backoff")
	maxDelay := flag.Duration("max-delay", 10*time.Second, "maximum delay for exponential backoff")
	serverID := flag.String("server-id", "", "Server ID (default derived from LFD ID)")
	serverAddr := flag.String("server-addr", "", "Server listen address to use when restarting replicas")
	backups := flag.String("backups", "", "Backup replicas for restarted server (e.g., S2=127.0.0.1:9002)")
	ckptMs := flag.Int("ckpt-ms", 5000, "Checkpoint interval (ms) for restarted server")
	startNewborn := flag.Bool("start-newborn", true, "Start recovered server as newborn replica")
	initState := flag.Int("init-state", 0, "Initial server state for recovered replicas")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	config := lfd.LFDConfig{
		LFDID:          *lfdID,
		TargetAddr:     *targetAddr,
		GFDAddr:        *gfdAddr,
		HBFreq:         *hb,
		Timeout:        *timeout,
		MaxRetries:     *maxRetries,
		BaseDelay:      *baseDelay,
		MaxDelay:       *maxDelay,
		ServerID:       *serverID,
		ServerAddr:     *serverAddr,
		Backups:        *backups,
		CkptMs:         *ckptMs,
		StartAsNewborn: *startNewborn,
		InitState:      *initState,
	}
	if config.ServerAddr == "" {
		config.ServerAddr = *targetAddr
	}

	l := lfd.NewLFDWithConfig(config)
	if err := l.Run(); err != nil {
		log.Fatal(err)
	}
}
