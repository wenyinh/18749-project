package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/wenyinh/18749-project/client"
)

// go run crunner.go -servers S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003 -id C1 -auto
func main() {
	servers := flag.String("servers", "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003", "server addresses (format: ID1=addr1,ID2=addr2,...)")
	clientID := flag.String("id", "C1", "client identifier")
	interval := flag.Duration("interval", 3*time.Second, "interval between requests")
	autoSend := flag.Bool("auto", false, "automatically send requests")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// Parse server addresses
	serverAddrs := parseServerAddrs(*servers)
	if len(serverAddrs) == 0 {
		log.Fatal("No server addresses provided")
	}

	// Create client
	c := client.NewClient(*clientID, serverAddrs)

	// Connect
	if err := c.Connect(); err != nil {
		log.Fatalf("Failed to connect to servers: %v", err)
	}
	defer c.Close()

	if *autoSend {
		// Auto mode: continuously send requests
		log.Printf("[%s] Starting auto-send mode (interval: %v)", *clientID, *interval)
		reqNum := 0
		for {
			reqNum++
			message := fmt.Sprintf("Auto request %d from %s", reqNum, *clientID)
			c.SendMessage(message)
			time.Sleep(*interval)
		}
	} else {
		// Manual mode: wait for user input
		fmt.Printf("Client %s connected. Commands:\n", *clientID)
		fmt.Println("  Type messages and press Enter to send")
		fmt.Println("  'quit' to exit")
		fmt.Println("  'auto' to switch to auto mode")

		var input string
		for {
			fmt.Print("> ")
			fmt.Scanln(&input)

			input = strings.TrimSpace(input)
			if input == "quit" {
				break
			}

			if input == "auto" {
				log.Printf("[%s] Switching to auto-send mode", *clientID)
				reqNum := 0
				for {
					reqNum++
					message := fmt.Sprintf("Auto request %d from %s", reqNum, *clientID)
					c.SendMessage(message)
					time.Sleep(*interval)
				}
			}

			if input != "" {
				c.SendMessage(input)
			}
		}
	}
}

func parseServerAddrs(servers string) map[string]string {
	result := make(map[string]string)
	pairs := strings.Split(servers, ",")
	for _, pair := range pairs {
		parts := strings.Split(pair, "=")
		if len(parts) == 2 {
			serverID := strings.TrimSpace(parts[0])
			addr := strings.TrimSpace(parts[1])
			result[serverID] = addr
		}
	}
	return result
}
