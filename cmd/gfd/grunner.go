package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/gfd"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8000", "GFD listen address")
	hb := flag.Duration("hb", 1*time.Second, "heartbeat frequency")
	timeout := flag.Duration("timeout", 3*time.Second, "heartbeat timeout")
	rmAddr := flag.String("rm_addr", "127.0.0.1:7000", "RM address")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	g := gfd.NewGFD(*addr, *rmAddr, *hb, *timeout)
	if err := g.Run(); err != nil {
		log.Fatal(err)
	}
}
