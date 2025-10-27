package server

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wenyinh/18749-project/utils"
)

const (
	Ping       = "PING"
	Pong       = "PONG"
	Req        = "REQ"
	Resp       = "RESP"
	Register   = "REGISTER"
	Ack        = "ACK"
	Nack       = "NACK"
	Checkpoint = "CHECKPOINT"
)

type Role int

const (
	Primary Role = iota
	Backup
)

type RequestMessage struct {
	Type       string `json:"type"`
	ClientID   string `json:"client_id"`
	RequestNum int    `json:"request_num"`
	Message    string `json:"message"`
}

type ResponseMessage struct {
	Type        string `json:"type"`
	ServerID    string `json:"server_id"`
	ClientID    string `json:"client_id"`
	RequestNum  int    `json:"request_num"`
	ServerState int    `json:"server_state"`
	Message     string `json:"message"`
}

type CheckpointMessage struct {
	Type          string `json:"type"`
	ReplicaId     string `json:"replica_id"`
	ServerState   int    `json:"server_state"`
	CheckpointNum int    `json:"checkpoint_num"`
}

type server struct {
	Addr           string
	ReplicaId      string
	ServerState    int
	ServerRole     Role
	Backups        map[string]string
	BackupConns    map[string]net.Conn
	CheckpointFreq time.Duration
	CheckpointNo   int
	mu             sync.Mutex
}

type MessageType struct {
	Type string `json:"type"`
}

func NewServer(
	addr, replicaId string,
	serverState int,
	role Role,
	backups map[string]string,
	backupConns map[string]net.Conn,
	ckptFreq time.Duration,
) Server {
	if backups == nil {
		backups = make(map[string]string)
	}
	if backupConns == nil {
		backupConns = make(map[string]net.Conn)
	}
	s := &server{
		Addr:           addr,
		ReplicaId:      replicaId,
		ServerState:    serverState,
		ServerRole:     role,
		Backups:        backups,
		BackupConns:    backupConns,
		CheckpointFreq: ckptFreq,
		CheckpointNo:   0,
	}
	return s
}

func (s *server) handleConnection(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	r := bufio.NewReader(conn)
	log.Printf("[SERVER][%s] connected to %s", s.ReplicaId, conn.RemoteAddr())

	isLFDConnection := false

	for {
		line, err := utils.ReadLine(r)
		if err != nil {
			if isLFDConnection {
				log.Printf("[SERVER][%s] LFD disconnected from %s", s.ReplicaId, conn.RemoteAddr())
			}
			return
		}

		// Check if this is a REGISTER message from LFD
		if len(line) >= len(Register) && line[:len(Register)] == Register {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				requestedServerID := parts[1]
				if requestedServerID == s.ReplicaId {
					// Server ID matches, acknowledge
					err := utils.WriteLine(conn, Ack)
					if err == nil {
						isLFDConnection = true
						log.Printf("[SERVER][%s] LFD registered successfully to monitor this server", s.ReplicaId)
					}
				} else {
					// Server ID mismatch, reject
					err := utils.WriteLine(conn, Nack)
					log.Printf("[SERVER][%s] rejected LFD registration: expected %s but got %s",
						s.ReplicaId, s.ReplicaId, requestedServerID)
					if err == nil {
						return
					}
				}
			}
			continue
		}

		if line == Ping {
			err := utils.WriteLine(conn, Pong)
			if err == nil {
				log.Printf("[SERVER][%s] heartbeat, sent pong to LFD", s.ReplicaId)
			}
			continue
		}

		var mt MessageType
		if err := json.Unmarshal([]byte(line), &mt); err != nil {
			log.Printf("[SERVER][%s] failed to parse JSON: %v", s.ReplicaId, err)
			_ = utils.WriteLine(conn, "ERROR: invalid JSON format")
			continue
		}

		switch mt.Type {
		case Req:
			var reqMsg RequestMessage
			if err := json.Unmarshal([]byte(line), &reqMsg); err != nil {
				log.Printf("[SERVER][%s] failed to parse JSON: %v", s.ReplicaId, err)
				_ = utils.WriteLine(conn, "ERROR: invalid JSON format")
				continue
			}
			if s.ServerRole == Backup {
				log.Printf("[SERVER][%s] is not primary server, skip handle request: client=%s, req_num=%d",
					s.ReplicaId, reqMsg.ClientID, reqMsg.RequestNum)
				continue
			}
			log.Printf("[SERVER][%s] received JSON request from client, clientId: %s, request_num: %d, Message: %s",
				s.ReplicaId, reqMsg.ClientID, reqMsg.RequestNum, reqMsg.Message)
			s.mu.Lock()
			replicaId := s.ReplicaId
			before := s.ServerState
			s.ServerState++
			after := s.ServerState
			s.mu.Unlock()
			log.Printf("[SERVER][%s] server state before: %d", replicaId, before)
			log.Printf("[SERVER][%s] server state after: %d", replicaId, after)
			// Create JSON response
			respMsg := ResponseMessage{
				Type:        Resp,
				ServerID:    replicaId,
				ClientID:    reqMsg.ClientID,
				RequestNum:  reqMsg.RequestNum,
				ServerState: after,
				Message:     reqMsg.Message,
			}
			jsonResp, err := json.Marshal(respMsg)
			if err != nil {
				log.Printf("[SERVER][%s] error marshaling response: %v", s.ReplicaId, err)
				_ = utils.WriteLine(conn, "ERROR: failed to create response")
				continue
			}
			_ = utils.WriteLine(conn, string(jsonResp))
			log.Printf("[SERVER][%s] sent JSON reply to client, clientId: %s, request_num: %d, server state: %d",
				s.ReplicaId, reqMsg.ClientID, reqMsg.RequestNum, s.ServerState)
		case Checkpoint:
			var ckpt CheckpointMessage
			if err := json.Unmarshal([]byte(line), &ckpt); err != nil {
				log.Printf("[SERVER][%s] bad CHECKPOINT json: %v", s.ReplicaId, err)
				continue
			}
			if s.ServerRole == Backup {
				s.mu.Lock()
				if ckpt.CheckpointNum <= s.CheckpointNo {
					s.mu.Unlock()
					log.Printf("[SERVER][%s] ignore stale checkpoint from %s: recv=%d <= local=%d",
						s.ReplicaId, ckpt.ReplicaId, ckpt.CheckpointNum, s.CheckpointNo)
					continue
				}
				s.ServerState = ckpt.ServerState
				s.CheckpointNo = ckpt.CheckpointNum
				s.mu.Unlock()
				log.Printf("[SERVER][%s] recv checkpoint from %s: server_state=%d, checkpoint_no=%d",
					s.ReplicaId, ckpt.ReplicaId, s.ServerState, s.CheckpointNo)
			}
		default:
			_ = utils.WriteLine(conn, "ERROR: unknown request type")
		}
	}
}

func (s *server) dialBackups() {
	if s.ServerRole != Primary {
		return
	}
	type target struct {
		id, addr string
	}
	var todo []target
	s.mu.Lock()
	for bid, baddr := range s.Backups {
		if _, ok := s.BackupConns[bid]; !ok {
			todo = append(todo, target{id: bid, addr: baddr})
		}
	}
	s.mu.Unlock()
	for _, t := range todo {
		conn, err := net.Dial("tcp", t.addr)
		if err != nil {
			log.Printf("[SERVER][%s] dial backup %s@%s failed: %v", s.ReplicaId, t.id, t.addr, err)
			continue
		}
		s.mu.Lock()
		if old, ok := s.BackupConns[t.id]; ok && old != nil {
			_ = conn.Close()
		} else {
			s.BackupConns[t.id] = conn
			log.Printf("[SERVER][%s] secondary channel established to backup %s@%s",
				s.ReplicaId, t.id, t.addr)
		}
		s.mu.Unlock()
	}
}

func (s *server) sendCheckpoint() {
	if s.ServerRole != Primary {
		return
	}
	s.mu.Lock()
	ckpt := CheckpointMessage{
		Type:          Checkpoint,
		ReplicaId:     s.ReplicaId,
		ServerState:   s.ServerState,
		CheckpointNum: s.CheckpointNo + 1,
	}
	s.CheckpointNo++
	conns := make(map[string]net.Conn, len(s.BackupConns))
	for id, c := range s.BackupConns {
		conns[id] = c
	}
	s.mu.Unlock()

	payload, err := json.Marshal(ckpt)
	if err != nil {
		log.Printf("[SERVER][%s] marshal checkpoint failed: %v", s.ReplicaId, err)
		return
	}
	line := string(payload)

	for bid, c := range conns {
		if c == nil {
			continue
		}
		if err := utils.WriteLine(c, line); err != nil {
			log.Printf("[SERVER][%s] send checkpoint to %s failed: %v (will drop conn)", s.ReplicaId, bid, err)
			s.mu.Lock()
			if old, ok := s.BackupConns[bid]; ok {
				_ = old.Close()
				delete(s.BackupConns, bid)
			}
			s.mu.Unlock()
		} else {
			log.Printf("[SERVER][%s] checkpoint #%d sent to %s (server_state=%d)",
				s.ReplicaId, ckpt.CheckpointNum, bid, ckpt.ServerState)
		}
	}
}

func (s *server) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if s.ServerRole == Primary && s.CheckpointFreq > 0 {
		go func() {
			t := time.NewTicker(s.CheckpointFreq)
			defer t.Stop()
			for range t.C {
				s.dialBackups()
				s.sendCheckpoint()
			}
		}()
	}
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
