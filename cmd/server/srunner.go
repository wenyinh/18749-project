package main

import (
	"flag"
	"log"

	"github.com/wenyinh/18749-project/server"
)

func main() {
	addr := flag.String("addr", ":9000", "server listen address, e.g. :9000 or 127.0.0.1:9000")
	rid := flag.String("rid", "S1", "replica id for logs")
	init := flag.Int("init_state", 0, "initial server state counter")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	s := server.NewServer(*addr, *rid, *init)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
