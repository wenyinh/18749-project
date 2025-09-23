package client

import (
	"github.com/wenyinh/18749-project/utils"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type client struct {
	clientID       int
	serverAddr     string
	conn           net.Conn
	reqID          int
	mu             sync.Mutex
	maxRetries     int
	baseDelay      time.Duration
	maxDelay       time.Duration
	currentRetries int
}

func NewClient(clientID int, serverAddr string) Client {
	return &client{
		clientID:   clientID,
		serverAddr: serverAddr,
		maxRetries: 5,
		baseDelay:  time.Second,
		maxDelay:   time.Minute,
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
			c.currentRetries = 0
			log.Printf("[C%d] Connected to server S1\n", c.clientID)
			return nil
		}

		if attempt == c.maxRetries {
			log.Printf("[C%d] Failed to connect after %d attempts: %v\n", c.clientID, c.maxRetries+1, err)
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

func (c *client) SendMessage() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		log.Printf("[C%d] Not connected to server, attempting to connect\n", c.clientID)
		if err := c.connectWithRetry(); err != nil {
			log.Printf("[C%d] Failed to establish connection: %v\n", c.clientID, err)
			return
		}
	}

	c.reqID++
	fullMsg := "REQ C" + strconv.Itoa(c.clientID) + " " + strconv.Itoa(c.reqID)

	log.Printf("[C%d] Sending request: %s\n", c.clientID, fullMsg)
	err := utils.WriteLine(c.conn, fullMsg)
	if err != nil {
		log.Printf("[C%d] Error sending message: %v\n", c.clientID, err)
		c.handleConnectionError()
		return
	}

	// Receive and print reply
	buffer := make([]byte, 1024)
	n, err := c.conn.Read(buffer)
	if err != nil {
		log.Printf("[C%d] Error receiving reply: %v\n", c.clientID, err)
		c.handleConnectionError()
		return
	}
	reply := string(buffer[:n])
	log.Printf("[C%d] Received reply: %s\n", c.clientID, reply)
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
