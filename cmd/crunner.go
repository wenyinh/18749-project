package main

import (
	"flag"
	"log"

	"github.com/wenyinh/18749-project/client"
)

func main() {
	id := flag.String("id", "1", "client ID (e.g., 1, 2, 3)")
	serverAddr := flag.String("server", "127.0.0.1:9000", "server address")
	testMode := flag.Bool("test", false, "run in test mode (sends periodic messages)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	c := client.NewClient(*id, *serverAddr, *testMode)
	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}
