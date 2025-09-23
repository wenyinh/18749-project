package server

import (
	"bufio"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/wenyinh/18749-project/utils"
)

const (
	Ping = "PING"
	Pong = "PONG"
	Req  = "REQ"
	Resp = "RESP"
)

type server struct {
	Addr        string
	ReplicaId   string
	ServerState int
}

func NewServer(addr, replicaId string, serverState int) Server {
	return &server{addr, replicaId, serverState}
}

func (s *server) handleConnection(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	r := bufio.NewReader(conn)
	log.Printf("[SERVER][%s] connected to %s", s.ReplicaId, conn.RemoteAddr())
	for {
		line, err := utils.ReadLine(r)
		if err != nil {
			return
		}
		if line == Ping {
			err := utils.WriteLine(conn, Pong)
			if err == nil {
				log.Printf("[SERVER][%s] heartbeat, sent pong to LFD", s.ReplicaId)
			}
		} else {
			// Echo back the exact message received
			log.Printf("[SERVER][%s] received message: %s", s.ReplicaId, line)
			_ = utils.WriteLine(conn, line)
			log.Printf("[SERVER][%s] echoed back message: %s", s.ReplicaId, line)
		}
	}
}

func (s *server) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	listener := utils.MustListen(s.Addr)
	log.Printf("[SERVER][%s] listening on %s", s.ReplicaId, s.Addr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[SERVER][%s] accept client error: %v", s.ReplicaId, err)
			continue
		}
		go s.handleConnection(conn)
	}
}
