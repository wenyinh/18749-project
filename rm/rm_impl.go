package rm

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

type rm struct {
	addr string

	membership  []string
	memberCount int
	gfdConn     net.Conn

	mu sync.Mutex
}

func NewRM(addr string) RM {
	return &rm{
		addr:       addr,
		membership: make([]string, 0),
	}
}

func (r *rm) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	l := utils.MustListen(r.addr)
	r.mu.Lock()
	r.printMembershipLocked()
	r.mu.Unlock()

	log.Printf("[RM] listening on %s", r.addr)

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("[RM] accept error: %v", err)
			continue
		}
		log.Printf("[RM] new connection from %s", conn.RemoteAddr())
		go r.handleConn(conn)
	}
}

func (r *rm) handleConn(conn net.Conn) {
	defer func() {
		log.Printf("[RM] connection %s closed", conn.RemoteAddr())
		_ = conn.Close()
	}()

	reader := bufio.NewReader(conn)

	line, err := utils.ReadLine(reader)
	if err != nil {
		log.Printf("[RM] initial read error from %s: %v", conn.RemoteAddr(), err)
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	if strings.HasPrefix(line, "MEMBERS") {
		r.handleGFD(conn, reader, line)
		return
	}

	log.Printf("[RM] ignore unknown first line from %s: %q", conn.RemoteAddr(), line)
}

func (r *rm) handleGFD(conn net.Conn, reader *bufio.Reader, first string) {
	log.Printf("[RM] connection %s identified as GFD", conn.RemoteAddr())

	r.mu.Lock()
	if r.gfdConn != nil && r.gfdConn != conn {
		_ = r.gfdConn.Close()
	}
	r.gfdConn = conn
	r.mu.Unlock()

	r.handleMembers(first)

	for {
		line, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[RM] GFD read error from %s: %v", conn.RemoteAddr(), err)
			r.mu.Lock()
			if r.gfdConn == conn {
				r.gfdConn = nil
			}
			r.mu.Unlock()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "MEMBERS") {
			r.handleMembers(line)
			continue
		}
		log.Printf("[RM] ignore unknown line from GFD %s: %q", conn.RemoteAddr(), line)
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

func (r *rm) updateMembership(m []string) {
	r.mu.Lock()
	old := append([]string(nil), r.membership...)
	r.membership = m
	r.memberCount = len(m)
	r.printMembershipLocked()
	r.mu.Unlock()

	failed := diffMembers(old, m)
	if len(failed) == 0 {
		return
	}

	log.Printf("[RM] detected %d replica(s) removed: %v", len(failed), failed)
	for _, sid := range failed {
		go r.recoverReplica(sid)
	}
}

func diffMembers(oldMembers, newMembers []string) []string {
	if len(oldMembers) == 0 {
		return nil
	}

	present := make(map[string]struct{}, len(newMembers))
	for _, sid := range newMembers {
		present[sid] = struct{}{}
	}

	var failed []string
	for _, sid := range oldMembers {
		if _, ok := present[sid]; !ok {
			failed = append(failed, sid)
		}
	}
	return failed
}

func (r *rm) recoverReplica(sid string) {
	log.Printf("[RM] scheduling recovery for %s", sid)
	time.Sleep(2 * time.Second)

	r.mu.Lock()
	conn := r.gfdConn
	r.mu.Unlock()

	if conn == nil {
		log.Printf("[RM] cannot recover %s: no GFD connection", sid)
		return
	}

	cmd := fmt.Sprintf("RECOVER %s", sid)
	log.Printf("[RM] sending %q to GFD", cmd)
	if err := utils.WriteLine(conn, cmd); err != nil {
		log.Printf("[RM] failed to send RECOVER for %s: %v", sid, err)
		r.mu.Lock()
		if r.gfdConn == conn {
			_ = conn.Close()
			r.gfdConn = nil
		}
		r.mu.Unlock()
		return
	}

	log.Printf("[RM] RECOVER command for %s sent", sid)
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
		fmt.Printf("%sRM: 1 member: %s%s\n",
			red, memberList, reset)
	} else {
		fmt.Printf("%sRM: %d members: %s%s\n",
			red, r.memberCount, memberList, reset)
	}
}
