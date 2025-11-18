package rm

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/wenyinh/18749-project/utils"
)

type rm struct {
	addr string

	membership  []string
	memberCount int

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

	for {
		line, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[RM] read error from %s: %v", conn.RemoteAddr(), err)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "MEMBERS") {
			r.handleMembers(line)
		} else {
			log.Printf("[RM] ignore unknown line from %s: %q", conn.RemoteAddr(), line)
		}
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

	r.mu.Lock()
	r.membership = m
	r.memberCount = len(m)
	r.printMembershipLocked()
	r.mu.Unlock()
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
