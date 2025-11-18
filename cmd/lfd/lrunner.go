package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/lfd"
)

// go run lrunner.go -target 127.0.0.1:9001 -gfd 172.26.1.83:8000 -id LFD1
func main() {
	targetAddr := flag.String("target", "127.0.0.1:9000", "server address to monitor")
	hb := flag.Duration("hb", 1*time.Second, "heartbeat frequency (e.g. 1s, 500ms)")
	timeout := flag.Duration("timeout", 3*time.Second, "heartbeat timeout (e.g. 3s)")
	lfdID := flag.String("id", "LFD1", "LFD identifier")
	gfdAddr := flag.String("gfd", "127.0.0.1:8000", "GFD address")
	maxRetries := flag.Int("max-retries", 3, "maximum reconnection attempts")
	baseDelay := flag.Duration("base-delay", 1*time.Second, "base delay for exponential backoff")
	maxDelay := flag.Duration("max-delay", 10*time.Second, "maximum delay for exponential backoff")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	l := lfd.NewLFD(*lfdID, *targetAddr, *gfdAddr, *hb, *timeout, *maxRetries, *baseDelay, *maxDelay)
	if err := l.Run(); err != nil {
		log.Fatal(err)
	}
}
