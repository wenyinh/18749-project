package gfd

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wenyinh/18749-project/utils"
)

const (
	gfdPing = "GFD_PING"
	gfdPong = "GFD_PONG"
)

type lfdInfo struct {
	lfdID      string
	serverID   string
	conn       net.Conn
	reader     *bufio.Reader
	lastHB     time.Time
	registered bool
}

type gfd struct {
	addr        string
	membership  []string // List of server IDs
	memberCount int
	serverToLFD map[string]string   // Map of server ID -> LFD ID
	lfdInfos    map[string]*lfdInfo // Map of LFD ID -> LFD info
	hbFreq      time.Duration       // Heartbeat frequency for GFD->LFD
	timeout     time.Duration       // Heartbeat timeout
	mu          sync.Mutex

	rmAddr string
	rmConn net.Conn
}

func NewGFD(addr string, rmAddr string, hbFreq, timeout time.Duration) GFD {
	return &gfd{
		addr:        addr,
		membership:  make([]string, 0),
		memberCount: 0,
		serverToLFD: make(map[string]string),
		lfdInfos:    make(map[string]*lfdInfo),
		hbFreq:      hbFreq,
		timeout:     timeout,
		rmAddr:      rmAddr,
	}
}

func (g *gfd) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	listener := utils.MustListen(g.addr)
	log.Printf("[GFD] listening on %s, heartbeat freq=%s, timeout=%s", g.addr, g.hbFreq, g.timeout)

	if err := g.connectRM(); err != nil {
		log.Printf("[GFD] WARNING: failed to connect RM at %s: %v", g.rmAddr, err)
	} else {
		g.notifyRM()
	}

	// Initial state
	g.printMembership()

	// Start heartbeat monitoring goroutine
	go g.heartbeatMonitor()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[GFD] accept error: %v", err)
			continue
		}
		go g.handleConnection(conn)
	}
}

func (g *gfd) handleConnection(conn net.Conn) {
	var lfdID string
	var info *lfdInfo

	defer func() {
		// When LFD disconnects, mark it as down
		if lfdID != "" {
			g.handleLFDDisconnection(lfdID)
		}
		_ = conn.Close()
	}()

	r := bufio.NewReader(conn)
	log.Printf("[GFD] LFD connected from %s", conn.RemoteAddr())

	for {
		line, err := utils.ReadLine(r)
		if err != nil {
			return
		}

		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}

		command := strings.ToUpper(parts[0])

		// Handle REGISTER command
		if command == "REGISTER" && len(parts) == 3 {
			serverID := parts[1]
			lfdID = parts[2]

			// Create LFD info and store connection
			g.mu.Lock()
			info = &lfdInfo{
				lfdID:      lfdID,
				serverID:   serverID,
				conn:       conn,
				reader:     r,
				lastHB:     time.Now(),
				registered: true,
			}
			g.lfdInfos[lfdID] = info
			g.mu.Unlock()

			log.Printf("[GFD] LFD %s registered to monitor server %s", lfdID, serverID)
			continue
		}

		// Handle GFD_PONG (heartbeat response from LFD)
		if command == "GFD_PONG" {
			log.Printf("[GFD] received GFD_PONG from %s", lfdID)
			g.mu.Lock()
			if info != nil {
				info.lastHB = time.Now()
			}
			g.mu.Unlock()
			continue
		}

		// Handle ADD/DELETE commands
		// ADD can be 2 or 3 parts: "ADD S1" or "ADD S1 LFD1"
		// DELETE should be 3 parts: "DELETE S1 LFD1"
		if command == "ADD" && len(parts) >= 2 {
			serverID := parts[1]
			// If LFD ID not provided in ADD message, use the registered LFD ID
			if len(parts) == 3 {
				lfdID = parts[2]
			}
			// Only add if we have a registered LFD for this connection
			if info != nil && info.registered {
				g.addReplica(serverID, info.lfdID)
			} else {
				log.Printf("[GFD] received ADD from unregistered LFD, ignoring")
			}
		} else if command == "DELETE" && len(parts) >= 2 {
			serverID := parts[1]
			if len(parts) == 3 {
				lfdID = parts[2]
			} else if info != nil {
				lfdID = info.lfdID
			}
			if lfdID != "" {
				g.deleteReplica(serverID, lfdID)
			} else {
				log.Printf("[GFD] received DELETE but cannot identify LFD")
			}
		} else if command != "ADD" && command != "DELETE" {
			log.Printf("[GFD] unknown command: %s", command)
		}
	}
}

func (g *gfd) handleLFDDisconnection(lfdID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	info, exists := g.lfdInfos[lfdID]
	if !exists {
		log.Printf("[GFD] LFD %s disconnected (not registered)", lfdID)
		return
	}

	serverID := info.serverID
	delete(g.lfdInfos, lfdID)

	log.Printf("[GFD] LFD %s disconnected (was monitoring server %s), NOT removing server from membership", lfdID, serverID)
}

// heartbeatMonitor periodically sends heartbeats to all registered LFDs
func (g *gfd) heartbeatMonitor() {
	ticker := time.NewTicker(g.hbFreq)
	defer ticker.Stop()

	for range ticker.C {
		g.mu.Lock()
		lfdsToCheck := make([]*lfdInfo, 0, len(g.lfdInfos))
		for _, info := range g.lfdInfos {
			if info.registered {
				lfdsToCheck = append(lfdsToCheck, info)
			}
		}
		g.mu.Unlock()

		// Send heartbeats to each LFD
		for _, info := range lfdsToCheck {
			go g.sendHeartbeatToLFD(info)
		}
	}
}

// sendHeartbeatToLFD sends a heartbeat to a specific LFD and checks for timeout
func (g *gfd) sendHeartbeatToLFD(info *lfdInfo) {
	// Check if last heartbeat response is too old
	g.mu.Lock()
	timeSinceLastHB := time.Since(info.lastHB)
	lfdID := info.lfdID
	serverID := info.serverID
	conn := info.conn
	g.mu.Unlock()

	if timeSinceLastHB > g.timeout {
		log.Printf("[GFD] LFD %s (monitoring %s) failed to respond to heartbeat (timeout=%s) <-- DETECTED LFD FAILURE",
			lfdID, serverID, g.timeout)

		// Remove LFD from tracking and delete server from membership
		g.handleLFDFailure(lfdID, serverID)
		return
	}

	// Send GFD_PING
	if conn != nil {
		err := utils.WriteLine(conn, gfdPing)
		log.Printf("[GFD] heartbeat to %s", lfdID)
		if err != nil {
			log.Printf("[GFD] failed to send heartbeat to LFD %s: %v", lfdID, err)
			g.handleLFDFailure(lfdID, serverID)
		}
	}
}

// handleLFDFailure is called when LFD fails to respond to heartbeats
func (g *gfd) handleLFDFailure(lfdID string, serverID string) {
	g.mu.Lock()

	// Remove LFD from tracking
	delete(g.lfdInfos, lfdID)

	// Remove server from membership
	found := false
	newMembership := make([]string, 0, len(g.membership))
	for _, member := range g.membership {
		if member != serverID {
			newMembership = append(newMembership, member)
		} else {
			found = true
		}
	}

	if found {
		g.membership = newMembership
		g.memberCount = len(g.membership)
		delete(g.serverToLFD, serverID)

		log.Printf("[GFD] removed server %s from membership due to LFD %s failure", serverID, lfdID)
		g.printMembershipLocked()
		g.mu.Unlock()

		g.notifyRM()
		return
	}

	g.mu.Unlock()
}

func (g *gfd) addReplica(serverID string, lfdID string) {
	g.mu.Lock()

	// Check if server already exists in membership
	for _, member := range g.membership {
		if member == serverID {
			log.Printf("[GFD] server %s already in membership (monitored by LFD %s)", serverID, lfdID)
			g.serverToLFD[serverID] = lfdID
			g.printMembershipLocked()
			g.mu.Unlock()
			g.notifyRM()
			return
		}
	}

	// Add server to membership
	g.membership = append(g.membership, serverID)
	g.memberCount = len(g.membership)
	g.serverToLFD[serverID] = lfdID

	log.Printf("[GFD] added server %s to membership (monitored by LFD %s)", serverID, lfdID)
	g.printMembershipLocked()
	g.mu.Unlock()

	g.notifyRM()
}

func (g *gfd) deleteReplica(serverID string, lfdID string) {
	g.mu.Lock()

	// Find and remove server from membership
	found := false
	newMembership := make([]string, 0, len(g.membership))
	for _, member := range g.membership {
		if member != serverID {
			newMembership = append(newMembership, member)
		} else {
			found = true
		}
	}

	if !found {
		log.Printf("[GFD] server %s not found in membership (DELETE request from LFD %s)", serverID, lfdID)
		g.mu.Unlock()
		return
	}

	g.membership = newMembership
	g.memberCount = len(g.membership)
	delete(g.serverToLFD, serverID)

	log.Printf("[GFD] deleted server %s from membership (reported by LFD %s)", serverID, lfdID)
	g.printMembershipLocked()
	g.mu.Unlock()

	g.notifyRM()
}

func (g *gfd) printMembership() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.printMembershipLocked()
}

func (g *gfd) printMembershipLocked() {
	red := "\033[31m"
	reset := "\033[0m"
	if g.memberCount == 0 {
		fmt.Printf("%sGFD: 0 members%s\n", red, reset)
	} else if g.memberCount == 1 {
		fmt.Printf("%sGFD: 1 member: %s%s\n", red, g.membership[0], reset)
	} else {
		memberList := strings.Join(g.membership, ", ")
		fmt.Printf("%sGFD: %d members: %s%s\n", red, g.memberCount, memberList, reset)
	}
}

func (g *gfd) connectRM() error {
	if g.rmAddr == "" {
		return fmt.Errorf("rmAddr is empty")
	}
	conn, err := net.Dial("tcp", g.rmAddr)
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.rmConn = conn
	g.mu.Unlock()
	log.Printf("[GFD] connected to RM at %s", g.rmAddr)
	return nil
}

func (g *gfd) notifyRM() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.rmConn == nil {
		return
	}

	var line string
	if g.memberCount == 0 {
		line = "MEMBERS"
	} else {
		memberList := strings.Join(g.membership, ",")
		line = fmt.Sprintf("MEMBERS %s", memberList)
	}

	if err := utils.WriteLine(g.rmConn, line); err != nil {
		log.Printf("[GFD] failed to notify RM (%q): %v", line, err)
		_ = g.rmConn.Close()
		g.rmConn = nil
	}
}
