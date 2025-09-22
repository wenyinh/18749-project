package main

import (
	"flag"
	"log"
	"time"
	"github.com/wenyinh/18749-project/client"
)


func main() {
	serverAddr := flag.String("server", "127.0.0.1:9000", "server address to connect")
	clientID := flag.Int("id", 1, "client identifier")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// create client
	c := client.NewClient(*clientID, *serverAddr)

	// connect
	if err := c.Connect(); err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer c.Close()
	

	for {
		c.SendMessage("hello world")
		time.Sleep(1 * time.Second)
	}
	
}

