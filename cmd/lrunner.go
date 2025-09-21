package main

import (
	"flag"
	"log"
	"strconv"

	"github.com/wenyinh/18749-project/lfd"
)

func main() {
	targetAddr := flag.String("target", "127.0.0.1:9000", "server address to monitor")
	intervalMs := flag.String("interval-ms", "1000", "heartbeat interval in milliseconds")
	lfdID := flag.String("id", "LFD1", "LFD identifier")
	flag.Parse()

	// Parse interval
	interval, err := strconv.Atoi(*intervalMs)
	if err != nil {
		log.Fatalf("[LFD] Invalid interval: %s", *intervalMs)
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	l := lfd.NewLfd(*lfdID, *targetAddr, interval)
	if err := l.Run(); err != nil {
		log.Fatal(err)
	}
}
