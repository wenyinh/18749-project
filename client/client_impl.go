package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/wenyinh/18749-project/utils"
)

type RequestMessage struct {
	Type       string `json:"type"`
	ClientID   string `json:"client_id"`
	RequestNum int    `json:"request_num"`
	Message    string `json:"message"`
}

type ResponseMessage struct {
	Type        string `json:"type"`
	ServerID    string `json:"server_id"`
	ClientID    string `json:"client_id"`
	RequestNum  int    `json:"request_num"`
	ServerState int    `json:"server_state"`
	Message     string `json:"message"`
}

type QueuedRequest struct {
	RequestNum int
	Message    string
	Timestamp  time.Time
}

type ReplicaConnection struct {
	ServerID        string
	Addr            string
	Conn            net.Conn
	IsHealthy       bool
	Queue           []QueuedRequest
	mu              sync.Mutex
	reader          *bufio.Reader
	reconnecting    bool // Flag to prevent multiple reconnection attempts
	permanentlyDown bool // Flag to mark replica as permanently unreachable
}

type client struct {
	clientID       string
	replicas       []*ReplicaConnection
	requestNum     int
	maxQueueSize   int
	maxRetries     int
	baseDelay      time.Duration
	mu             sync.Mutex
	pendingReplies map[int]bool // Track which requests have been delivered
	replyMu        sync.Mutex
}

func NewClient(clientID string, serverAddrs map[string]string) Client {
	replicas := make([]*ReplicaConnection, 0, len(serverAddrs))
	for serverID, addr := range serverAddrs {
		replicas = append(replicas, &ReplicaConnection{
			ServerID:  serverID,
			Addr:      addr,
			IsHealthy: false,
			Queue:     make([]QueuedRequest, 0),
		})
	}

	return &client{
		clientID:       clientID,
		replicas:       replicas,
		requestNum:     0,
		maxQueueSize:   100,
		maxRetries:     5,
		baseDelay:      time.Second,
		pendingReplies: make(map[int]bool),
	}
}

func (c *client) Connect() error {
	log.Printf("[%s] Connecting to all replicas...", c.clientID)

	var wg sync.WaitGroup
	errors := make([]error, len(c.replicas))

	for i, replica := range c.replicas {
		wg.Add(1)
		go func(idx int, r *ReplicaConnection) {
			defer wg.Done()
			err := c.connectReplica(r)
			errors[idx] = err
			if err == nil {
				log.Printf("[%s→%s] Connected successfully", c.clientID, r.ServerID)
			} else {
				log.Printf("[%s→%s] Initial connection failed: %v", c.clientID, r.ServerID, err)
				// Start background reconnection
				go c.attemptReconnect(r)
			}
		}(i, replica)
	}

	wg.Wait()

	// Check if at least one replica is connected
	hasConnection := false
	for _, replica := range c.replicas {
		if replica.IsHealthy {
			hasConnection = true
			break
		}
	}

	if !hasConnection {
		return fmt.Errorf("failed to connect to any replica")
	}

	return nil
}

func (c *client) connectReplica(replica *ReplicaConnection) error {
	replica.mu.Lock()
	defer replica.mu.Unlock()

	conn, err := net.Dial("tcp", replica.Addr)
	if err != nil {
		return err
	}

	replica.Conn = conn
	replica.reader = bufio.NewReader(conn)
	replica.IsHealthy = true
	return nil
}

func (c *client) SendMessage(message string) {
	c.mu.Lock()
	c.requestNum++
	reqNum := c.requestNum
	c.mu.Unlock()

	req := QueuedRequest{
		RequestNum: reqNum,
		Message:    message,
		Timestamp:  time.Now(),
	}

	log.Printf("[%s] Sending request_num=%d to all replicas", c.clientID, reqNum)

	// Channel to collect responses
	responseChan := make(chan ResponseMessage, len(c.replicas))
	var wg sync.WaitGroup

	// Send to all replicas
	for _, replica := range c.replicas {
		wg.Add(1)
		go func(r *ReplicaConnection) {
			defer wg.Done()
			c.sendToReplica(r, req, responseChan)
		}(replica)
	}

	// Wait for responses in a separate goroutine
	go func() {
		wg.Wait()
		close(responseChan)
	}()

	// Process responses
	firstReply := true
	for resp := range responseChan {
		c.replyMu.Lock()
		if firstReply {
			firstReply = false
			c.pendingReplies[reqNum] = true
			c.replyMu.Unlock()
			log.Printf("[%s←%s] Received reply for request_num=%d (state=%d)",
				c.clientID, resp.ServerID, resp.RequestNum, resp.ServerState)
		} else {
			c.replyMu.Unlock()
			log.Printf("[%s←%s] request_num %d: Discarded duplicate reply from %s",
				c.clientID, resp.ServerID, resp.RequestNum, resp.ServerID)
		}
	}
}

func (c *client) sendToReplica(replica *ReplicaConnection, req QueuedRequest, responseChan chan ResponseMessage) {
	replica.mu.Lock()

	// Check if replica is permanently down - skip it entirely
	if replica.permanentlyDown {
		replica.mu.Unlock()
		log.Printf("[%s→%s] Replica permanently down, skipping request_num=%d",
			c.clientID, replica.ServerID, req.RequestNum)
		return
	}

	if !replica.IsHealthy || replica.Conn == nil {
		// Queue the request (already holding lock, so call internal version)
		c.enqueueRequestLocked(replica, req)
		queueSize := len(replica.Queue)
		replica.mu.Unlock()
		log.Printf("[%s→%s] Connection down, queued request_num=%d (queue size: %d)",
			c.clientID, replica.ServerID, req.RequestNum, queueSize)
		go c.attemptReconnect(replica)
		return
	}

	conn := replica.Conn
	reader := replica.reader
	replica.mu.Unlock()

	// Construct JSON request
	reqMsg := RequestMessage{
		Type:       "REQ",
		ClientID:   c.clientID,
		RequestNum: req.RequestNum,
		Message:    req.Message,
	}

	jsonData, err := json.Marshal(reqMsg)
	if err != nil {
		log.Printf("[%s→%s] Error marshaling JSON: %v", c.clientID, replica.ServerID, err)
		return
	}

	log.Printf("[%s→%s] Sending request_num=%d", c.clientID, replica.ServerID, req.RequestNum)

	// Send request
	err = utils.WriteLine(conn, string(jsonData))
	if err != nil {
		log.Printf("[%s→%s] Error sending request: %v", c.clientID, replica.ServerID, err)
		c.markUnhealthy(replica)
		c.enqueueRequest(replica, req)
		go c.attemptReconnect(replica)
		return
	}

	// Receive response with timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reply, err := utils.ReadLine(reader)
	if err != nil {
		log.Printf("[%s→%s] Error receiving reply: %v", c.clientID, replica.ServerID, err)
		c.markUnhealthy(replica)
		c.enqueueRequest(replica, req)
		go c.attemptReconnect(replica)
		return
	}

	// Parse JSON response
	var respMsg ResponseMessage
	if err := json.Unmarshal([]byte(reply), &respMsg); err == nil {
		responseChan <- respMsg
	} else {
		log.Printf("[%s→%s] Failed to parse response: %v", c.clientID, replica.ServerID, err)
	}
}

func (c *client) enqueueRequest(replica *ReplicaConnection, req QueuedRequest) {
	replica.mu.Lock()
	defer replica.mu.Unlock()
	c.enqueueRequestLocked(replica, req)
}

// enqueueRequestLocked adds a request to the queue without acquiring the lock
// Caller must hold replica.mu.Lock()
func (c *client) enqueueRequestLocked(replica *ReplicaConnection, req QueuedRequest) {
	if len(replica.Queue) >= c.maxQueueSize {
		log.Printf("[%s→%s] Queue full (%d), dropping oldest request",
			c.clientID, replica.ServerID, c.maxQueueSize)
		replica.Queue = replica.Queue[1:]
	}

	replica.Queue = append(replica.Queue, req)
}

func (c *client) markUnhealthy(replica *ReplicaConnection) {
	replica.mu.Lock()
	defer replica.mu.Unlock()

	replica.IsHealthy = false
	if replica.Conn != nil {
		replica.Conn.Close()
		replica.Conn = nil
		replica.reader = nil
	}
}

func (c *client) attemptReconnect(replica *ReplicaConnection) {
	// Check if already healthy or another goroutine is reconnecting
	replica.mu.Lock()
	if replica.IsHealthy {
		replica.mu.Unlock()
		return
	}
	if replica.reconnecting {
		// Another goroutine is already trying to reconnect
		replica.mu.Unlock()
		return
	}
	// Mark as reconnecting to prevent multiple concurrent attempts
	replica.reconnecting = true
	replica.mu.Unlock()

	// Ensure we clear the reconnecting flag when done
	defer func() {
		replica.mu.Lock()
		replica.reconnecting = false
		replica.mu.Unlock()
	}()

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		delay := c.calculateBackoffDelay(attempt)
		log.Printf("[%s→%s] Reconnecting in %v (attempt %d/%d)...",
			c.clientID, replica.ServerID, delay, attempt+1, c.maxRetries)
		time.Sleep(delay)

		err := c.connectReplica(replica)
		if err == nil {
			replica.mu.Lock()
			queueSize := len(replica.Queue)
			replica.mu.Unlock()

			log.Printf("[%s→%s] Reconnected successfully, flushing %d queued requests",
				c.clientID, replica.ServerID, queueSize)
			c.flushQueue(replica)
			return
		}

		log.Printf("[%s→%s] Reconnection attempt %d failed: %v",
			c.clientID, replica.ServerID, attempt+1, err)
	}

	log.Printf("[%s→%s] Reconnection failed after %d attempts, replica marked as permanently down",
		c.clientID, replica.ServerID, c.maxRetries)

	// Mark replica as permanently down so future requests skip it
	replica.mu.Lock()
	replica.permanentlyDown = true
	replica.mu.Unlock()
}

func (c *client) flushQueue(replica *ReplicaConnection) {
	replica.mu.Lock()
	queue := make([]QueuedRequest, len(replica.Queue))
	copy(queue, replica.Queue)
	replica.Queue = make([]QueuedRequest, 0)
	replica.mu.Unlock()

	for _, req := range queue {
		log.Printf("[%s→%s] Sending queued request_num=%d (queued for %v)",
			c.clientID, replica.ServerID, req.RequestNum, time.Since(req.Timestamp))

		// Create a dummy response channel (we don't wait for responses during flush)
		responseChan := make(chan ResponseMessage, 1)
		c.sendToReplica(replica, req, responseChan)
		close(responseChan)

		// Drain the channel
		for range responseChan {
		}
	}
}

func (c *client) calculateBackoffDelay(attempt int) time.Duration {
	delay := time.Duration(1<<uint(attempt)) * c.baseDelay
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}

func (c *client) Close() {
	log.Printf("[%s] Closing all connections", c.clientID)
	for _, replica := range c.replicas {
		replica.mu.Lock()
		if replica.Conn != nil {
			replica.Conn.Close()
			replica.Conn = nil
			replica.reader = nil
		}
		replica.IsHealthy = false
		replica.mu.Unlock()
	}
}
