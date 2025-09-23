package client

import (
	"encoding/json"
	"github.com/wenyinh/18749-project/utils"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

type RequestMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
	Message  string `json:"message"`
}

type ResponseMessage struct {
	Type        string `json:"type"`
	ServerID    string `json:"server_id"`
	ClientID    string `json:"client_id"`
	ServerState int    `json:"server_state"`
	Message     string `json:"message"`
}

type client struct {
	clientID           int
	serverAddr         string
	conn               net.Conn
	reqID              int
	mu                 sync.Mutex
	maxRetries         int
	baseDelay          time.Duration
	maxDelay           time.Duration
	consecutiveFailures int
	maxConsecutiveFailures int
}

func NewClient(clientID int, serverAddr string) Client {
	return &client{
		clientID:               clientID,
		serverAddr:             serverAddr,
		maxRetries:             5,
		baseDelay:              time.Second,
		maxDelay:               time.Minute,
		maxConsecutiveFailures: 3,
	}
}

func (c *client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectWithRetry()
}

func (c *client) connectWithRetry() error {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		conn, err := net.Dial("tcp", c.serverAddr)
		if err == nil {
			c.conn = conn
			c.consecutiveFailures = 0
			log.Printf("[C%d] Connected to server S1\n", c.clientID)
			return nil
		}

		if attempt == c.maxRetries {
			log.Printf("[C%d] Failed to connect after %d attempts: %v\n", c.clientID, c.maxRetries+1, err)
			c.consecutiveFailures++
			return err
		}

		delay := c.calculateBackoffDelay(attempt)
		log.Printf("[C%d] Connection attempt %d failed, retrying in %v: %v\n", c.clientID, attempt+1, delay, err)
		time.Sleep(delay)
	}
	return nil
}

func (c *client) calculateBackoffDelay(attempt int) time.Duration {
	delay := time.Duration(1<<uint(attempt)) * c.baseDelay
	if delay > c.maxDelay {
		delay = c.maxDelay
	}
	return delay
}

func (c *client) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	log.Printf("[C%d] Attempting to reconnect...\n", c.clientID)
	return c.connectWithRetry()
}

func (c *client) SendMessage(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		log.Printf("[C%d] Not connected to server, attempting to connect\n", c.clientID)
		if err := c.connectWithRetry(); err != nil {
			log.Printf("[C%d] Failed to establish connection after all retries, exiting: %v\n", c.clientID, err)
			os.Exit(1)
		}
	}

	// Construct JSON request message
	reqMsg := RequestMessage{
		Type:     "REQ",
		ClientID: "C" + strconv.Itoa(c.clientID),
		Message:  message,
	}

	jsonData, err := json.Marshal(reqMsg)
	if err != nil {
		log.Printf("[C%d] Error marshaling JSON: %v\n", c.clientID, err)
		return
	}

	log.Printf("[C%d] Sending JSON message: %s\n", c.clientID, string(jsonData))
	err = utils.WriteLine(c.conn, string(jsonData))
	if err != nil {
		log.Printf("[C%d] Error sending message: %v\n", c.clientID, err)
		c.handleConnectionError()
		if err := c.connectWithRetry(); err != nil {
			log.Printf("[C%d] Failed to reconnect after all retries, exiting: %v\n", c.clientID, err)
			os.Exit(1)
		}
		return
	}

	// Receive and parse JSON reply
	buffer := make([]byte, 1024)
	n, err := c.conn.Read(buffer)
	if err != nil {
		log.Printf("[C%d] Error receiving reply: %v\n", c.clientID, err)
		c.handleConnectionError()
		if err := c.connectWithRetry(); err != nil {
			log.Printf("[C%d] Failed to reconnect after all retries, exiting: %v\n", c.clientID, err)
			os.Exit(1)
		}
		return
	}
	reply := string(buffer[:n])
	log.Printf("[C%d] Received JSON reply: %s\n", c.clientID, reply)

	// Try to parse as JSON response
	var respMsg ResponseMessage
	if err := json.Unmarshal(buffer[:n], &respMsg); err == nil {
		log.Printf("[C%d] Parsed response - Server: %s, State: %d, Message: %s\n",
			c.clientID, respMsg.ServerID, respMsg.ServerState, respMsg.Message)
	}
}

func (c *client) handleConnectionError() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	log.Printf("[C%d] Connection lost, will attempt to reconnect on next message\n", c.clientID)
}

func (c *client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		log.Printf("[C%d] Disconnected from server S1\n", c.clientID)
	}
}
