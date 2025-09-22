package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/lfd"
)

func main() {
	targetAddr := flag.String("target", "127.0.0.1:9000", "server address to monitor")
	hb := flag.Duration("hb", 1*time.Second, "heartbeat frequency (e.g. 1s, 500ms)")
	timeout := flag.Duration("timeout", 3*time.Second, "heartbeat timeout (e.g. 3s)")
	lfdID := flag.String("id", "LFD1", "LFD identifier")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)


	l := lfd.NewLFD(*lfdID, *targetAddr, *hb, *timeout)
	if err := l.Run(); err != nil {
		log.Fatal(err)
	}
}
