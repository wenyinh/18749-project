package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
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
	reconnecting    bool
	permanentlyDown bool
}

type client struct {
	clientID       string
	replicas       []*ReplicaConnection
	primaryID      string
	requestNum     int
	maxQueueSize   int
	maxRetries     int
	baseDelay      time.Duration
	mu             sync.Mutex
	pendingReplies map[int]bool
	replyMu        sync.Mutex

	rmAddr string
	rmConn net.Conn
	rmMu   sync.Mutex
}

func NewClient(clientID string, serverAddrs map[string]string, primaryID string, rmAddr string) Client {
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
		primaryID:      primaryID,
		requestNum:     0,
		maxQueueSize:   100,
		maxRetries:     5,
		baseDelay:      time.Second,
		pendingReplies: make(map[int]bool),
		rmAddr:         rmAddr,
	}
}

func (c *client) Connect() error {
	log.Printf("[%s] Connecting to all replicas...", c.clientID)

	if c.rmAddr != "" {
		go c.connectToRM()
	}

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
				go c.attemptReconnect(r)
			}
		}(i, replica)
	}

	wg.Wait()

	hasConnection := false
	for _, replica := range c.replicas {
		if replica.IsHealthy {
			hasConnection = true
			break
		}
	}

	if !hasConnection && c.rmAddr == "" {
		return fmt.Errorf("failed to connect to any replica")
	}

	if !hasConnection && c.rmAddr != "" {
		log.Printf("[%s] no initial replica connection, will rely on RM for primary and reconnect in background", c.clientID)
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
	replica.permanentlyDown = false
	return nil
}

func (c *client) activeReplicas() []*ReplicaConnection {
	targets := make([]*ReplicaConnection, 0, len(c.replicas))
	for _, r := range c.replicas {
		r.mu.Lock()
		down := r.permanentlyDown
		r.mu.Unlock()
		if !down {
			targets = append(targets, r)
		}
	}
	return targets
}

func (c *client) SendMessage(message string) {
	c.mu.Lock()
	c.requestNum++
	reqNum := c.requestNum
	primary := c.primaryID
	c.mu.Unlock()

	if primary == "" {
		log.Printf("[%s] no primary selected yet, drop request_num=%d", c.clientID, reqNum)
		return
	}

	req := QueuedRequest{
		RequestNum: reqNum,
		Message:    message,
		Timestamp:  time.Now(),
	}

	var targets []*ReplicaConnection
	for _, r := range c.activeReplicas() {
		if r.ServerID == primary {
			targets = append(targets, r)
			break
		}
	}
	if len(targets) == 0 {
		log.Printf("[%s] no active replica for primary=%s, drop request_num=%d", c.clientID, primary, reqNum)
		return
	}

	log.Printf("[%s] Sending request_num=%d to primary=%s", c.clientID, reqNum, primary)

	responseChan := make(chan ResponseMessage, len(targets))
	var wg sync.WaitGroup

	for _, replica := range targets {
		wg.Add(1)
		go func(r *ReplicaConnection) {
			defer wg.Done()
			c.sendToReplica(r, req, responseChan)
		}(replica)
	}

	go func() {
		wg.Wait()
		close(responseChan)
	}()

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

	if replica.permanentlyDown {
		replica.mu.Unlock()
		log.Printf("[%s→%s] Replica permanently down, skipping request_num=%d",
			c.clientID, replica.ServerID, req.RequestNum)
		return
	}

	if !replica.IsHealthy || replica.Conn == nil {
		c.enqueueRequestLocked(replica, req)
		queueSize := len(replica.Queue)
		permDown := replica.permanentlyDown
		replica.mu.Unlock()
		if permDown {
			return
		}
		log.Printf("[%s→%s] Connection down, queued request_num=%d (queue size: %d)",
			c.clientID, replica.ServerID, req.RequestNum, queueSize)
		go c.attemptReconnect(replica)
		return
	}

	conn := replica.Conn
	reader := replica.reader
	replica.mu.Unlock()

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

	err = utils.WriteLine(conn, string(jsonData))
	if err != nil {
		log.Printf("[%s→%s] Error sending request: %v", c.clientID, replica.ServerID, err)
		c.markUnhealthy(replica)
		c.enqueueRequest(replica, req)
		go c.attemptReconnect(replica)
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reply, err := utils.ReadLine(reader)
	if err != nil {
		log.Printf("[%s→%s] Error receiving reply: %v", c.clientID, replica.ServerID, err)
		c.markUnhealthy(replica)
		c.enqueueRequest(replica, req)
		go c.attemptReconnect(replica)
		return
	}

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
	if replica.permanentlyDown {
		log.Printf("[%s→%s] Permanently down; drop queued request_num=%d",
			c.clientID, replica.ServerID, req.RequestNum)
		return
	}
	c.enqueueRequestLocked(replica, req)
}

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
	replica.mu.Lock()
	if replica.permanentlyDown {
		replica.mu.Unlock()
		return
	}
	if replica.IsHealthy {
		replica.mu.Unlock()
		return
	}
	if replica.reconnecting {
		replica.mu.Unlock()
		return
	}
	replica.reconnecting = true
	replica.mu.Unlock()

	defer func() {
		replica.mu.Lock()
		replica.reconnecting = false
		replica.mu.Unlock()
	}()

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		replica.mu.Lock()
		if replica.permanentlyDown {
			replica.mu.Unlock()
			return
		}
		replica.mu.Unlock()

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

		responseChan := make(chan ResponseMessage, 1)
		c.sendToReplica(replica, req, responseChan)
		close(responseChan)
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

func (c *client) connectToRM() {
	log.Printf("[%s] connecting to RM at %s...", c.clientID, c.rmAddr)

	conn, err := net.Dial("tcp", c.rmAddr)
	if err != nil {
		log.Printf("[%s] failed to connect to RM: %v", c.clientID, err)
		return
	}

	c.rmMu.Lock()
	c.rmConn = conn
	c.rmMu.Unlock()

	hello := fmt.Sprintf("HELLO_CLIENT %s", c.clientID)
	if err := utils.WriteLine(conn, hello); err != nil {
		log.Printf("[%s] failed to send HELLO_CLIENT: %v", c.clientID, err)
		_ = conn.Close()
		c.rmMu.Lock()
		c.rmConn = nil
		c.rmMu.Unlock()
		return
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[%s] RM connection closed: %v", c.clientID, err)
			c.rmMu.Lock()
			if c.rmConn == conn {
				c.rmConn = nil
			}
			c.rmMu.Unlock()
			_ = conn.Close()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		if strings.ToUpper(parts[0]) != "PRIMARY" {
			continue
		}
		newID := parts[1]
		newAddr := ""
		if len(parts) >= 3 {
			newAddr = parts[2]
		}
		c.handlePrimaryChange(newID, newAddr)
	}
}

func (c *client) handlePrimaryChange(newPrimaryID, newPrimaryAddr string) {
	c.mu.Lock()
	old := c.primaryID
	c.primaryID = newPrimaryID

	var target *ReplicaConnection
	for _, r := range c.replicas {
		if r.ServerID == newPrimaryID {
			target = r
			break
		}
	}
	if target == nil && newPrimaryAddr != "" {
		target = &ReplicaConnection{
			ServerID:  newPrimaryID,
			Addr:      newPrimaryAddr,
			IsHealthy: false,
			Queue:     make([]QueuedRequest, 0),
		}
		c.replicas = append(c.replicas, target)
	}

	if target != nil && newPrimaryAddr != "" {
		target.mu.Lock()
		if target.Addr != newPrimaryAddr {
			target.Addr = newPrimaryAddr
			if target.Conn != nil {
				target.Conn.Close()
				target.Conn = nil
				target.reader = nil
			}
			target.IsHealthy = false
		}
		target.mu.Unlock()
	}

	c.mu.Unlock()

	if newPrimaryID == "" {
		log.Printf("[%s] RM cleared primary", c.clientID)
		return
	}

	if newPrimaryID != old {
		log.Printf("[%s] RM changed primary: %s -> %s (%s)", c.clientID, old, newPrimaryID, newPrimaryAddr)
	} else {
		log.Printf("[%s] RM confirmed primary: %s (%s)", c.clientID, newPrimaryID, newPrimaryAddr)
	}
	if target != nil {
		target.mu.Lock()
		if target.permanentlyDown {
			log.Printf("[%s] RM set %s as PRIMARY, clear permanentlyDown flag", c.clientID, target.ServerID)
			target.permanentlyDown = false
		}
		healthy := target.IsHealthy
		target.mu.Unlock()
		if !healthy {
			go c.attemptReconnect(target)
		}
	}
}

func (c *client) Close() {
	log.Printf("[%s] Closing all connections", c.clientID)

	c.rmMu.Lock()
	if c.rmConn != nil {
		c.rmConn.Close()
		c.rmConn = nil
	}
	c.rmMu.Unlock()

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
