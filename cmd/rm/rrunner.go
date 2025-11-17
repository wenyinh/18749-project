package main

import (
	"flag"
	"log"

	"github.com/wenyinh/18749-project/rm"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:7000", "RM listen address")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	r := rm.NewRM(*addr)
	if err := r.Run(); err != nil {
		log.Fatal(err)
	}
}
