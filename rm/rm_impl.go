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
	addr        string
	membership  []string
	memberCount int
	primaryID   string

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
	log.Printf("[RM] listening on %s", r.addr)

	r.printMembershipLocked()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("[RM] accept error: %v", err)
			continue
		}
		go r.handleConnection(conn)
	}
}

func (r *rm) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	log.Printf("[RM] connection from %s", conn.RemoteAddr())

	for {
		line, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[RM] connection %s closed: %v", conn.RemoteAddr(), err)
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToUpper(parts[0])

		if cmd != "MEMBERS" {
			log.Printf("[RM] unknown command: %s (line=%q)", cmd, line)
			continue
		}

		var members []string
		if len(parts) == 1 {
			members = []string{}
		} else {
			// MEMBERS S1,S2,S3
			for _, m := range strings.Split(parts[1], ",") {
				m = strings.TrimSpace(m)
				if m != "" {
					members = append(members, m)
				}
			}
		}

		r.updateMembership(members)
	}
}

func (r *rm) updateMembership(members []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.membership = members
	r.memberCount = len(members)
	r.choosePrimaryLocked()
	r.printMembershipLocked()
}

func (r *rm) choosePrimaryLocked() {
	if len(r.membership) == 0 {
		r.primaryID = ""
		return
	}
	minID := r.membership[0]
	for _, id := range r.membership[1:] {
		if id < minID {
			minID = id
		}
	}
	r.primaryID = minID
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
