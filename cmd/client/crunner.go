package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/wenyinh/18749-project/client"
	"log"
	"os"
	"strings"
)

func main() {
	serverAddr := flag.String("server", "172.26.11.183:9000", "server address to connect")
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

	// Start interactive input loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("Client %d connected. Type messages and press Enter to send (type 'quit' to exit):\n", *clientID)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "quit" {
			break
		}

		if input != "" {
			c.SendMessage(input)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading input: %v", err)
	}

}
