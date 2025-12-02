package main

import (
	"flag"
	"log"

	"github.com/wenyinh/18749-project/rm"
)

// go run cmd/rm/rrunner.go -addr 127.0.0.1:8001
func main() {
	addr := flag.String("addr", ":8001", "RM listen address")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	r := rm.NewRM(*addr)
	if err := r.Run(); err != nil {
		log.Fatal(err)
	}
}
