package gfd

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/wenyinh/18749-project/utils"
)

type gfd struct {
	addr         string
	membership   []string          // List of server IDs
	memberCount  int
	serverToLFD  map[string]string // Map of server ID -> LFD ID
	lfdConns     map[string]net.Conn // Map of LFD ID -> connection
	mu           sync.Mutex
}

func NewGFD(addr string) GFD {
	return &gfd{
		addr:        addr,
		membership:  make([]string, 0),
		memberCount: 0,
		serverToLFD: make(map[string]string),
		lfdConns:    make(map[string]net.Conn),
	}
}

func (g *gfd) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	listener := utils.MustListen(g.addr)
	log.Printf("[GFD] listening on %s", g.addr)

	// Initial state
	g.printMembership()

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
	defer func() {
		// When LFD disconnects, log it but DON'T remove server from membership
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
		if len(parts) < 3 {
			log.Printf("[GFD] invalid message (expected 'CMD SERVER_ID LFD_ID'): %s", line)
			continue
		}

		command := strings.ToUpper(parts[0])
		serverID := parts[1]
		lfdID = parts[2]

		// Store connection for this LFD
		g.mu.Lock()
		g.lfdConns[lfdID] = conn
		g.mu.Unlock()

		switch command {
		case "ADD":
			g.addReplica(serverID, lfdID)
		case "DELETE":
			g.deleteReplica(serverID, lfdID)
		default:
			log.Printf("[GFD] unknown command: %s", command)
		}
	}
}

func (g *gfd) handleLFDDisconnection(lfdID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Find which server this LFD was monitoring
	var serverID string
	for sID, lID := range g.serverToLFD {
		if lID == lfdID {
			serverID = sID
			break
		}
	}

	// Remove connection
	delete(g.lfdConns, lfdID)

	if serverID != "" {
		log.Printf("[GFD] LFD %s disconnected (was monitoring server %s), but NOT removing server from membership", lfdID, serverID)
	} else {
		log.Printf("[GFD] LFD %s disconnected", lfdID)
	}
}

func (g *gfd) addReplica(serverID string, lfdID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check if server already exists in membership
	for _, member := range g.membership {
		if member == serverID {
			log.Printf("[GFD] server %s already in membership (monitored by LFD %s)", serverID, lfdID)
			g.serverToLFD[serverID] = lfdID
			return
		}
	}

	// Add server to membership
	g.membership = append(g.membership, serverID)
	g.memberCount = len(g.membership)
	g.serverToLFD[serverID] = lfdID

	log.Printf("[GFD] added server %s to membership (monitored by LFD %s)", serverID, lfdID)
	g.printMembershipLocked()
}

func (g *gfd) deleteReplica(serverID string, lfdID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

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
		return
	}

	g.membership = newMembership
	g.memberCount = len(g.membership)
	delete(g.serverToLFD, serverID)

	log.Printf("[GFD] deleted server %s from membership (reported by LFD %s)", serverID, lfdID)
	g.printMembershipLocked()
}

func (g *gfd) printMembership() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.printMembershipLocked()
}

func (g *gfd) printMembershipLocked() {
	if g.memberCount == 0 {
		fmt.Printf("GFD: 0 members\n")
	} else {
		memberList := strings.Join(g.membership, ", ")
		fmt.Printf("GFD: %d members: %s\n", g.memberCount, memberList)
	}
}
