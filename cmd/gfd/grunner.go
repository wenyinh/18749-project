package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/gfd"
)

// go run grunner.go -addr 0.0.0.0:8000 -rm 127.0.0.1:8001
func main() {
	addr := flag.String("addr", ":8000", "GFD listen address")
	rmAddr := flag.String("rm", "127.0.0.1:8001", "RM address")
	hbFreq := flag.Duration("hb", 1*time.Second, "Heartbeat frequency for GFD->LFD")
	timeout := flag.Duration("timeout", 3*time.Second, "Heartbeat timeout")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	g := gfd.NewGFD(*addr, *rmAddr, *hbFreq, *timeout)
	if err := g.Run(); err != nil {
		log.Fatal(err)
	}
}
