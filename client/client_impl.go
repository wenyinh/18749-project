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
	Type        string `json:"type"`
	ClientID    string `json:"client_id"`
	RequestNum  int    `json:"request_num"`
	Message     string `json:"message"`
	GlobalState int    `json:"global_state"`
}

type ResponseMessage struct {
	Type        string `json:"type"`
	ServerID    string `json:"server_id"`
	ClientID    string `json:"client_id"`
	RequestNum  int    `json:"request_num"`
	ServerState int    `json:"server_state"`
	Message     string `json:"message"`
}

type ReplicaConnection struct {
	ServerID string
	Addr     string

	Conn      net.Conn
	IsHealthy bool
	reader    *bufio.Reader

	mu              sync.Mutex
	reconnecting    bool
	permanentlyDown bool
}

type client struct {
	clientID string

	replicas []*ReplicaConnection

	requestNum     int
	maxRetries     int
	baseDelay      time.Duration
	mu             sync.Mutex
	pendingReplies map[int]bool
	replyMu        sync.Mutex

	globalState int
	gsMu        sync.Mutex
}

func NewClient(clientID string, serverAddrs map[string]string) Client {
	replicas := make([]*ReplicaConnection, 0, len(serverAddrs))
	for serverID, addr := range serverAddrs {
		replicas = append(replicas, &ReplicaConnection{
			ServerID:  serverID,
			Addr:      addr,
			IsHealthy: false,
		})
	}

	return &client{
		clientID:       clientID,
		replicas:       replicas,
		requestNum:     0,
		maxRetries:     5,
		baseDelay:      time.Second,
		pendingReplies: make(map[int]bool),
	}
}

func (c *client) getGlobalState() int {
	c.gsMu.Lock()
	defer c.gsMu.Unlock()
	return c.globalState
}

func (c *client) updateGlobalState(v int) {
	c.gsMu.Lock()
	if v > c.globalState {
		c.globalState = v
	}
	c.gsMu.Unlock()
}

func (c *client) Connect() error {
	log.Printf("[%s] Connecting to all replicas...", c.clientID)

	var wg sync.WaitGroup
	errs := make([]error, len(c.replicas))

	for i, replica := range c.replicas {
		wg.Add(1)
		go func(idx int, r *ReplicaConnection) {
			defer wg.Done()
			err := c.connectReplica(r)
			errs[idx] = err
			if err == nil {
				log.Printf("[%s→%s] Connected successfully", c.clientID, r.ServerID)
			} else {
				log.Printf("[%s→%s] Initial connection failed: %v", c.clientID, r.ServerID, err)
				go c.attemptReconnect(r)
			}
		}(i, replica)
	}

	wg.Wait()

	has := false
	for _, r := range c.replicas {
		r.mu.Lock()
		ok := r.IsHealthy
		r.mu.Unlock()
		if ok {
			has = true
			break
		}
	}
	if !has {
		return fmt.Errorf("failed to connect to any replica")
	}
	return nil
}

func (c *client) connectReplica(r *ReplicaConnection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, err := net.Dial("tcp", r.Addr)
	if err != nil {
		return err
	}
	r.Conn = conn
	r.reader = bufio.NewReader(conn)
	r.IsHealthy = true
	return nil
}

func (c *client) activeReplicas() []*ReplicaConnection {
	result := make([]*ReplicaConnection, 0, len(c.replicas))
	for _, r := range c.replicas {
		r.mu.Lock()
		down := r.permanentlyDown
		r.mu.Unlock()
		if !down {
			result = append(result, r)
		}
	}
	return result
}

func (c *client) SendMessage(message string) {
	c.mu.Lock()
	c.requestNum++
	reqNum := c.requestNum
	c.mu.Unlock()

	targets := c.activeReplicas()
	if len(targets) == 0 {
		log.Printf("[%s] No active replicas (all permanently down). Drop request_num=%d",
			c.clientID, reqNum)
		return
	}

	log.Printf("[%s] Sending request_num=%d to %d replicas (global_state=%d)",
		c.clientID, reqNum, len(targets), c.getGlobalState())

	responseChan := make(chan ResponseMessage, len(targets))
	var wg sync.WaitGroup

	for _, r := range targets {
		wg.Add(1)
		go func(rep *ReplicaConnection) {
			defer wg.Done()
			c.sendToReplica(rep, reqNum, message, responseChan)
		}(r)
	}

	go func() {
		wg.Wait()
		close(responseChan)
	}()

	firstReply := true
	for resp := range responseChan {
		// 所有响应都用来更新 global_state
		c.updateGlobalState(resp.ServerState)

		c.replyMu.Lock()
		if firstReply {
			firstReply = false
			c.pendingReplies[reqNum] = true
			c.replyMu.Unlock()
			log.Printf("[%s←%s] Received reply for request_num=%d (state=%d)",
				c.clientID, resp.ServerID, resp.RequestNum, resp.ServerState)
		} else {
			c.replyMu.Unlock()
			log.Printf(
				"[%s←%s] request_num=%d: Discard duplicate reply from %s (state=%d)",
				c.clientID,
				resp.ServerID,
				resp.RequestNum,
				resp.ServerID,
				resp.ServerState,
			)
		}
	}
}

func (c *client) sendToReplica(r *ReplicaConnection, reqNum int, msg string, ch chan<- ResponseMessage) {
	r.mu.Lock()
	if r.permanentlyDown {
		r.mu.Unlock()
		log.Printf("[%s→%s] Replica permanently down, skip request_num=%d",
			c.clientID, r.ServerID, reqNum)
		return
	}
	if !r.IsHealthy || r.Conn == nil {
		r.mu.Unlock()
		log.Printf("[%s→%s] Connection down, skip request_num=%d and try reconnect",
			c.clientID, r.ServerID, reqNum)
		go c.attemptReconnect(r)
		return
	}
	conn := r.Conn
	reader := r.reader
	r.mu.Unlock()

	req := RequestMessage{
		Type:        "REQ",
		ClientID:    c.clientID,
		RequestNum:  reqNum,
		Message:     msg,
		GlobalState: c.getGlobalState(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		log.Printf("[%s→%s] Error marshaling JSON: %v", c.clientID, r.ServerID, err)
		return
	}

	log.Printf("[%s→%s] Sending request_num=%d (global_state=%d)",
		c.clientID, r.ServerID, reqNum, req.GlobalState)

	if err := utils.WriteLine(conn, string(data)); err != nil {
		log.Printf("[%s→%s] Error sending request: %v", c.clientID, r.ServerID, err)
		c.markUnhealthy(r)
		go c.attemptReconnect(r)
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := utils.ReadLine(reader)
	if err != nil {
		log.Printf("[%s→%s] Error receiving reply: %v", c.clientID, r.ServerID, err)
		c.markUnhealthy(r)
		go c.attemptReconnect(r)
		return
	}

	var resp ResponseMessage
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		log.Printf("[%s→%s] Failed to parse response: %v", c.clientID, r.ServerID, err)
		return
	}

	ch <- resp
}

func (c *client) markUnhealthy(r *ReplicaConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.IsHealthy = false
	if r.Conn != nil {
		_ = r.Conn.Close()
		r.Conn = nil
		r.reader = nil
	}
}

func (c *client) attemptReconnect(r *ReplicaConnection) {
	r.mu.Lock()
	if r.permanentlyDown {
		r.mu.Unlock()
		return
	}
	if r.IsHealthy {
		r.mu.Unlock()
		return
	}
	if r.reconnecting {
		r.mu.Unlock()
		return
	}
	r.reconnecting = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.reconnecting = false
		r.mu.Unlock()
	}()

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		r.mu.Lock()
		if r.permanentlyDown {
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()

		delay := c.calculateBackoffDelay(attempt)
		log.Printf("[%s→%s] Reconnecting in %v (attempt %d/%d)...",
			c.clientID, r.ServerID, delay, attempt+1, c.maxRetries)
		time.Sleep(delay)

		if err := c.connectReplica(r); err == nil {
			log.Printf("[%s→%s] Reconnected successfully", c.clientID, r.ServerID)
			return
		}
	}

	log.Printf("[%s→%s] Reconnection failed after %d attempts; will retry on future sends",
		c.clientID, r.ServerID, c.maxRetries)
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
	for _, r := range c.replicas {
		r.mu.Lock()
		if r.Conn != nil {
			_ = r.Conn.Close()
			r.Conn = nil
			r.reader = nil
		}
		r.IsHealthy = false
		r.mu.Unlock()
	}
}
