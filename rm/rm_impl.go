package rm

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/wenyinh/18749-project/utils"
)

type rm struct {
	addr        string
	serverAddrs map[string]string

	membership  []string
	memberCount int
	primaryID   string

	clients map[string]net.Conn
	servers map[string]net.Conn

	serverStates map[string]int

	mu sync.Mutex
}

func NewRM(addr string, serverAddrs map[string]string) RM {
	return &rm{
		addr:         addr,
		serverAddrs:  serverAddrs,
		membership:   make([]string, 0),
		clients:      make(map[string]net.Conn),
		servers:      make(map[string]net.Conn),
		serverStates: make(map[string]int),
	}
}

func (r *rm) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	l := utils.MustListen(r.addr)
	log.Printf("[RM] listening on %s", r.addr)

	r.mu.Lock()
	r.printMembershipLocked()
	r.mu.Unlock()

	for {
		c, err := l.Accept()
		if err != nil {
			log.Printf("[RM] accept error: %v", err)
			continue
		}
		log.Printf("[RM] new connection from %s", c.RemoteAddr())
		go r.dispatch(c)
	}
}

func (r *rm) dispatch(conn net.Conn) {
	reader := bufio.NewReader(conn)
	line, err := utils.ReadLine(reader)
	if err != nil {
		_ = conn.Close()
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		_ = conn.Close()
		return
	}
	parts := strings.Fields(line)
	switch strings.ToUpper(parts[0]) {
	case "MEMBERS":
		log.Printf("[RM] connection %s identified as GFD", conn.RemoteAddr())
		r.handleGFD(conn, reader, line)
	case "HELLO_CLIENT":
		log.Printf("[RM] connection %s identified as client (%s)", conn.RemoteAddr(), line)
		r.handleClient(conn, reader, parts)
	case "HELLO_SERVER":
		log.Printf("[RM] connection %s identified as server (%s)", conn.RemoteAddr(), line)
		r.handleServer(conn, reader, parts)
	default:
		log.Printf("[RM] unknown first command from %s: %q", conn.RemoteAddr(), line)
		_ = conn.Close()
	}
}

func (r *rm) handleGFD(conn net.Conn, reader *bufio.Reader, first string) {
	defer func() {
		log.Printf("[RM] GFD connection %s closed", conn.RemoteAddr())
		conn.Close()
	}()
	r.handleMembers(first)
	for {
		line, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[RM] GFD read error from %s: %v", conn.RemoteAddr(), err)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "MEMBERS") {
			log.Printf("[RM] ignore unknown line from GFD %s: %q", conn.RemoteAddr(), line)
			continue
		}
		r.handleMembers(line)
	}
}

func (r *rm) handleMembers(line string) {
	log.Printf("[RM] received MEMBERS line: %q", line)
	parts := strings.Fields(line)
	var m []string
	if len(parts) > 1 {
		for _, x := range strings.Split(parts[1], ",") {
			x = strings.TrimSpace(x)
			if x != "" {
				m = append(m, x)
			}
		}
	}
	r.updateMembership(m)
}

func (r *rm) handleClient(conn net.Conn, reader *bufio.Reader, parts []string) {
	if len(parts) != 2 {
		log.Printf("[RM] invalid HELLO_CLIENT line from %s: %v", conn.RemoteAddr(), parts)
		_ = conn.Close()
		return
	}
	cid := parts[1]
	r.mu.Lock()
	if old, ok := r.clients[cid]; ok {
		log.Printf("[RM] closing previous connection for client %s", cid)
		_ = old.Close()
	}
	r.clients[cid] = conn
	pid := r.primaryID
	paddr := r.serverAddrs[pid]
	r.mu.Unlock()

	log.Printf("[RM] client %s registered from %s", cid, conn.RemoteAddr())

	if pid != "" && paddr != "" {
		line := fmt.Sprintf("PRIMARY %s %s", pid, paddr)
		log.Printf("[RM] send initial PRIMARY to client %s: %s", cid, line)
		if err := utils.WriteLine(conn, line); err != nil {
			log.Printf("[RM] failed to send PRIMARY to client %s: %v", cid, err)
			_ = conn.Close()
			r.mu.Lock()
			delete(r.clients, cid)
			r.mu.Unlock()
			return
		}
	}
	for {
		_, err := reader.ReadByte()
		if err != nil {
			log.Printf("[RM] client %s disconnected: %v", cid, err)
			r.mu.Lock()
			delete(r.clients, cid)
			r.mu.Unlock()
			_ = conn.Close()
			return
		}
	}
}

func (r *rm) handleServer(conn net.Conn, reader *bufio.Reader, parts []string) {
	if len(parts) != 2 {
		log.Printf("[RM] invalid HELLO_SERVER line from %s: %v", conn.RemoteAddr(), parts)
		_ = conn.Close()
		return
	}
	sid := parts[1]

	r.mu.Lock()
	if old, ok := r.servers[sid]; ok {
		log.Printf("[RM] closing previous connection for server %s", sid)
		_ = old.Close()
	}
	r.servers[sid] = conn

	role := "BACKUP"
	if r.primaryID != "" && r.primaryID == sid {
		role = "PRIMARY"
	}
	line := fmt.Sprintf("ROLE %s", role)
	r.mu.Unlock()

	log.Printf("[RM] server %s registered from %s, send initial ROLE: %s", sid, conn.RemoteAddr(), line)
	if err := utils.WriteLine(conn, line); err != nil {
		log.Printf("[RM] failed to send ROLE to server %s: %v", sid, err)
		_ = conn.Close()
		r.mu.Lock()
		delete(r.servers, sid)
		r.mu.Unlock()
		return
	}

	for {
		l, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[RM] server %s disconnected: %v", sid, err)
			r.mu.Lock()
			delete(r.servers, sid)
			r.mu.Unlock()
			_ = conn.Close()
			return
		}
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		fs := strings.Fields(l)
		switch strings.ToUpper(fs[0]) {
		case "STATE":
			if len(fs) < 3 {
				log.Printf("[RM] bad STATE line from %s: %q", sid, l)
				continue
			}
			stateSid := fs[1]
			val, err := strconv.Atoi(fs[2])
			if err != nil {
				log.Printf("[RM] bad STATE value from %s: %q", sid, l)
				continue
			}
			r.updateState(stateSid, val)
		default:
			log.Printf("[RM] ignore unknown line from server %s: %q", sid, l)
		}
	}
}

func (r *rm) updateMembership(m []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.membership = m
	r.memberCount = len(m)

	alive := make(map[string]struct{}, len(m))
	for _, sid := range m {
		alive[sid] = struct{}{}
	}
	for sid := range r.serverStates {
		if _, ok := alive[sid]; !ok {
			log.Printf("[RM] remove state of non-member server %s (old state=%d)", sid, r.serverStates[sid])
			delete(r.serverStates, sid)
		}
	}
	r.reevaluatePrimaryLocked("membership")
}

func (r *rm) updateState(sid string, state int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	old := r.serverStates[sid]
	if state <= old {
		return
	}
	r.serverStates[sid] = state

	log.Printf("[RM] update state: %s: %d -> %d", sid, old, state)
	r.reevaluatePrimaryLocked("state")
}

func (r *rm) choosePrimaryLocked() {
	if len(r.membership) == 0 {
		r.primaryID = ""
		return
	}

	current := r.primaryID

	bestID := ""
	bestState := -1

	inMembership := false
	for _, sid := range r.membership {
		if sid == current {
			inMembership = true
			break
		}
	}

	if inMembership && current != "" {
		bestID = current
		bestState = r.getStateLocked(current)
	}

	for _, sid := range r.membership {
		st := r.getStateLocked(sid)
		if bestID == "" || st > bestState {
			bestID = sid
			bestState = st
		}
	}

	r.primaryID = bestID
}

func (r *rm) reevaluatePrimaryLocked(reason string) {
	old := r.primaryID
	r.choosePrimaryLocked()
	now := r.primaryID

	if old != now {
		log.Printf("[RM] primary changed by %s: %q -> %q", reason, old, now)
	} else {
		log.Printf("[RM] primary unchanged after %s: %q", reason, now)
	}
	r.printMembershipLocked()

	if now != "" && now != old {
		addr := r.serverAddrs[now]
		line := fmt.Sprintf("PRIMARY %s %s", now, addr)
		log.Printf("[RM] broadcasting new PRIMARY to clients: %s", line)
		for cid, c := range r.clients {
			if c == nil {
				continue
			}
			if err := utils.WriteLine(c, line); err != nil {
				log.Printf("[RM] failed to send PRIMARY to client %s: %v", cid, err)
				_ = c.Close()
				r.clients[cid] = nil
			}
		}
	}

	for sid, c := range r.servers {
		if c == nil {
			continue
		}
		role := "BACKUP"
		if now != "" && sid == now {
			role = "PRIMARY"
		}
		rline := fmt.Sprintf("ROLE %s", role)
		log.Printf("[RM] send ROLE to server %s: %s", sid, rline)
		if err := utils.WriteLine(c, rline); err != nil {
			log.Printf("[RM] failed to send ROLE to server %s: %v", sid, err)
			_ = c.Close()
			r.servers[sid] = nil
		}
	}
}

func (r *rm) printMembershipLocked() {
	red := "\033[31m"
	reset := "\033[0m"

	if r.memberCount == 0 {
		fmt.Printf("%sRM: 0 members%s\n", red, reset)
		return
	}

	memberList := strings.Join(r.membership, ", ")
	if r.memberCount == 1 {
		fmt.Printf("%sRM: 1 member: %s (primary=%s)%s\n",
			red, memberList, r.primaryID, reset)
	} else {
		fmt.Printf("%sRM: %d members: %s (primary=%s)%s\n",
			red, r.memberCount, memberList, r.primaryID, reset)
	}
}

func (r *rm) getStateLocked(sid string) int {
	if st, ok := r.serverStates[sid]; ok {
		return st
	}
	return -1
}
