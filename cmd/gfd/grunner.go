package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/gfd"
)

// go run grunner.go -addr 0.0.0.0:8000
func main() {
	addr := flag.String("addr", ":8000", "GFD listen address")
	hbFreq := flag.Duration("hb", 1*time.Second, "Heartbeat frequency for GFD->LFD")
	timeout := flag.Duration("timeout", 3*time.Second, "Heartbeat timeout")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	g := gfd.NewGFD(*addr, *hbFreq, *timeout)
	if err := g.Run(); err != nil {
		log.Fatal(err)
	}
}
