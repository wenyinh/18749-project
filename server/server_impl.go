package server

import (
	"bufio"
	"github.com/wenyinh/18749-project/utils"
	"log"
	"net"
	"strconv"
	"strings"
)

const (
	Ping = "PING"
	Pong = "PONG"
	Req  = "REQ"
	Resp = "RESP"
)

type server struct {
	replicaId   string
	serverState int
}

func NewServer(replicaId string, serverState int) Server {
	return &server{replicaId, serverState}
}

func (s *server) handleConnection(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	r := bufio.NewReader(conn)
	log.Printf("[SERVER][%s] connected to %s", s.replicaId, conn.RemoteAddr())
	for {
		line, err := utils.ReadLine(r)
		if err != nil {
			return
		}
		if line == Ping {
			err := utils.WriteLine(conn, Pong)
			if err == nil {
				log.Printf("[SERVER][%s] heartbeat, sent pong to LFD", s.replicaId)
			}
		} else if strings.HasPrefix(line, Req) {
			// Client sent: REQ <client_id> <req_id>
			parts := strings.Split(line, " ")
			if len(parts) != 3 {
				_ = utils.WriteLine(conn, "ERROR: invalid request format")
			}
			clientId := parts[1]
			requestId := parts[2]
			log.Printf("[SERVER][%s] received request from client, clientId: %s, request ID: %s", s.replicaId, clientId, requestId)
			log.Printf("[SERVER][%s] state before: %d", s.replicaId, s.serverState)
			s.serverState++
			log.Printf("[SERVER][%s] state after: %d", s.replicaId, s.serverState)
			// RSP <clientId> <reqId> <serverState>
			_ = utils.WriteLine(conn, Resp+clientId+" "+requestId+" "+strconv.Itoa(s.serverState))
			log.Printf("[SERVER][%s] reply to client, clientId: %s, requestId: %s, server state: %d", s.replicaId, clientId, requestId, s.serverState)
		} else {
			_ = utils.WriteLine(conn, "ERROR: unknown request")
		}
	}
}

func (s *server) Run(addr, replicaId string) error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	s.replicaId = replicaId
	listener := utils.MustListen(addr)
	log.Printf("[SERVER][%s] listening on %s", s.replicaId, addr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[SERVER][%s] accept client error: %v", replicaId, err)
			continue
		}
		go s.handleConnection(conn)
	}
}
