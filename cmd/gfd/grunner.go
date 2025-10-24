package main

import (
	"flag"
	"log"

	"github.com/wenyinh/18749-project/gfd"
)

func main() {
	addr := flag.String("addr", ":8000", "GFD listen address")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	g := gfd.NewGFD(*addr)
	if err := g.Run(); err != nil {
		log.Fatal(err)
	}
}
