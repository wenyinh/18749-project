package server

import (
	"bufio"
	"encoding/json"
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

type RequestMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
	Message  string `json:"message"`
}

type ResponseMessage struct {
	Type        string `json:"type"`
	ServerID    string `json:"server_id"`
	ClientID    string `json:"client_id"`
	ServerState int    `json:"server_state"`
	Message     string `json:"message"`
}

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
			// Try to parse as JSON message
			var reqMsg RequestMessage
			if err := json.Unmarshal([]byte(line), &reqMsg); err != nil {
				log.Printf("[SERVER][%s] failed to parse JSON: %v", s.ReplicaId, err)
				_ = utils.WriteLine(conn, "ERROR: invalid JSON format")
				continue
			}

			if reqMsg.Type == Req {
				log.Printf("[SERVER][%s] received JSON request from client, clientId: %s, Message: %s", s.ReplicaId, reqMsg.ClientID, reqMsg.Message)
				log.Printf("[SERVER][%s] server state before: %d", s.ReplicaId, s.ServerState)
				s.ServerState++
				log.Printf("[SERVER][%s] server state after: %d", s.ReplicaId, s.ServerState)

				// Create JSON response
				respMsg := ResponseMessage{
					Type:        Resp,
					ServerID:    s.ReplicaId,
					ClientID:    reqMsg.ClientID,
					ServerState: s.ServerState,
					Message:     reqMsg.Message,
				}

				jsonResp, err := json.Marshal(respMsg)
				if err != nil {
					log.Printf("[SERVER][%s] error marshaling response: %v", s.ReplicaId, err)
					_ = utils.WriteLine(conn, "ERROR: failed to create response")
					continue
				}

				_ = utils.WriteLine(conn, string(jsonResp))
				log.Printf("[SERVER][%s] sent JSON reply to client, clientId: %s, server state: %d, message: %s", s.ReplicaId, reqMsg.ClientID, s.ServerState, reqMsg.Message)
			} else {
				_ = utils.WriteLine(conn, "ERROR: unknown request type")
			}
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
