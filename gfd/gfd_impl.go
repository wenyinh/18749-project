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
	membership   []string
	memberCount  int
	mu           sync.Mutex
}

func NewGFD(addr string) GFD {
	return &gfd{
		addr:        addr,
		membership:  make([]string, 0),
		memberCount: 0,
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
	defer func() {
		_ = conn.Close()
	}()

	r := bufio.NewReader(conn)
	log.Printf("[GFD] connected to %s", conn.RemoteAddr())

	for {
		line, err := utils.ReadLine(r)
		if err != nil {
			return
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			log.Printf("[GFD] invalid message: %s", line)
			continue
		}

		command := strings.ToUpper(parts[0])
		replicaID := parts[1]

		switch command {
		case "ADD":
			g.addReplica(replicaID)
		case "DELETE":
			g.deleteReplica(replicaID)
		default:
			log.Printf("[GFD] unknown command: %s", command)
		}
	}
}

func (g *gfd) addReplica(replicaID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check if already exists
	for _, member := range g.membership {
		if member == replicaID {
			log.Printf("[GFD] replica %s already in membership", replicaID)
			return
		}
	}

	g.membership = append(g.membership, replicaID)
	g.memberCount = len(g.membership)

	log.Printf("[GFD] added replica %s", replicaID)
	g.printMembershipLocked()
}

func (g *gfd) deleteReplica(replicaID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Find and remove
	found := false
	newMembership := make([]string, 0, len(g.membership))
	for _, member := range g.membership {
		if member != replicaID {
			newMembership = append(newMembership, member)
		} else {
			found = true
		}
	}

	if !found {
		log.Printf("[GFD] replica %s not found in membership", replicaID)
		return
	}

	g.membership = newMembership
	g.memberCount = len(g.membership)

	log.Printf("[GFD] deleted replica %s", replicaID)
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
