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
		} else if strings.HasPrefix(line, Req) {
			// Client sent: REQ <client_id> <Message>
			parts := strings.Split(line, " ")
			if len(parts) != 3 {
				_ = utils.WriteLine(conn, "ERROR: invalid request format")
				continue
			}
			clientId := parts[1]
			msg := parts[2]
			log.Printf("[SERVER][%s] received request from client, clientId: %s, Message: %s", s.ReplicaId, clientId, msg)
			log.Printf("[SERVER][%s] server state before: %d", s.ReplicaId, s.ServerState)
			s.ServerState++
			log.Printf("[SERVER][%s] server state after: %d", s.ReplicaId, s.ServerState)
			// RESP <serverId> <clientId> <ServerState> <Message>
			_ = utils.WriteLine(conn, Resp+" "+s.ReplicaId+" "+clientId+" "+" "+strconv.Itoa(s.ServerState)+" "+msg)
			log.Printf("[SERVER][%s] reply to client, clientId: %s, server state: %d, message: %s", s.ReplicaId, clientId, s.ServerState, msg)
		} else {
			_ = utils.WriteLine(conn, "ERROR: unknown request")
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
