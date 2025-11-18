package server

import (
	"bufio"
	"encoding/json"
	"fmt"
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

	rmAddr string
	rmConn net.Conn
	rmMu   sync.Mutex

	mu sync.Mutex
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
	rmAddr string,
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
		rmAddr:         rmAddr,
	}
	return s
}

func (s *server) notifyStateToRM() {
	if s.rmAddr == "" {
		return
	}

	s.rmMu.Lock()
	conn := s.rmConn
	s.rmMu.Unlock()
	if conn == nil {
		return
	}

	s.mu.Lock()
	state := s.ServerState
	sid := s.ReplicaId
	s.mu.Unlock()

	line := fmt.Sprintf("STATE %s %d", sid, state)
	if err := utils.WriteLine(conn, line); err != nil {
		log.Printf("[SERVER][%s] failed to send STATE to RM: %v", s.ReplicaId, err)
		s.rmMu.Lock()
		if s.rmConn == conn {
			_ = conn.Close()
			s.rmConn = nil
		}
		s.rmMu.Unlock()
	} else {
		log.Printf("[SERVER][%s] reported state=%d to RM", s.ReplicaId, state)
	}
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

		if len(line) >= len(Register) && line[:len(Register)] == Register {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				requestedServerID := parts[1]
				if requestedServerID == s.ReplicaId {
					err := utils.WriteLine(conn, Ack)
					if err == nil {
						isLFDConnection = true
						log.Printf("[SERVER][%s] LFD registered successfully to monitor this server", s.ReplicaId)
					}
				} else {
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

			s.notifyStateToRM()

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
			s.mu.Lock()
			role := s.ServerRole
			if role == Backup {
				if ckpt.CheckpointNum <= s.CheckpointNo {
					oldNo := s.CheckpointNo
					s.mu.Unlock()
					log.Printf("[SERVER][%s] ignore stale checkpoint from %s: recv=%d <= local=%d",
						s.ReplicaId, ckpt.ReplicaId, ckpt.CheckpointNum, oldNo)
					continue
				}
				s.ServerState = ckpt.ServerState
				s.CheckpointNo = ckpt.CheckpointNum
				newState := s.ServerState
				newNo := s.CheckpointNo
				s.mu.Unlock()

				log.Printf("\033[34m[SERVER][%s] recv checkpoint from %s: server_state=%d, checkpoint_no=%d\033[0m",
					s.ReplicaId, ckpt.ReplicaId, newState, newNo)

				s.notifyStateToRM()
			} else {
				s.mu.Unlock()
				log.Printf("[SERVER][%s] received CHECKPOINT while in PRIMARY role, ignoring", s.ReplicaId)
			}

		default:
			_ = utils.WriteLine(conn, "ERROR: unknown request type")
		}
	}
}

func (s *server) dialBackups() {
	s.mu.Lock()
	role := s.ServerRole
	backups := s.Backups
	existing := s.BackupConns
	s.mu.Unlock()

	if role != Primary {
		return
	}

	type target struct {
		id, addr string
	}
	var todo []target

	for bid, baddr := range backups {
		if _, ok := existing[bid]; !ok {
			todo = append(todo, target{id: bid, addr: baddr})
		}
	}

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
	s.mu.Lock()
	if s.ServerRole != Primary {
		s.mu.Unlock()
		return
	}
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
			log.Printf("\033[34m[SERVER][%s] checkpoint #%d sent to %s (server_state=%d)\033[0m",
				s.ReplicaId, ckpt.CheckpointNum, bid, ckpt.ServerState)
		}
	}
}

func (s *server) connectRM() {
	if s.rmAddr == "" {
		return
	}
	conn, err := net.Dial("tcp", s.rmAddr)
	if err != nil {
		log.Printf("[SERVER][%s] failed to connect RM %s: %v", s.ReplicaId, s.rmAddr, err)
		return
	}
	s.rmMu.Lock()
	s.rmConn = conn
	s.rmMu.Unlock()

	hello := fmt.Sprintf("HELLO_SERVER %s", s.ReplicaId)
	if err := utils.WriteLine(conn, hello); err != nil {
		log.Printf("[SERVER][%s] failed to send HELLO_SERVER to RM: %v", s.ReplicaId, err)
		_ = conn.Close()
		s.rmMu.Lock()
		s.rmConn = nil
		s.rmMu.Unlock()
		return
	}

	log.Printf("[SERVER][%s] connected to RM %s", s.ReplicaId, s.rmAddr)

	reader := bufio.NewReader(conn)
	for {
		line, err := utils.ReadLine(reader)
		if err != nil {
			log.Printf("[SERVER][%s] RM connection closed: %v", s.ReplicaId, err)
			s.rmMu.Lock()
			if s.rmConn == conn {
				s.rmConn = nil
			}
			s.rmMu.Unlock()
			_ = conn.Close()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		if strings.ToUpper(parts[0]) != "ROLE" {
			continue
		}
		roleStr := strings.ToUpper(parts[1])

		s.mu.Lock()
		old := s.ServerRole
		if roleStr == "PRIMARY" {
			s.ServerRole = Primary
		} else {
			s.ServerRole = Backup
		}
		newRole := s.ServerRole
		s.mu.Unlock()

		if old != newRole {
			log.Printf("[SERVER][%s] role changed by RM: %v -> %v", s.ReplicaId, old, newRole)
		} else {
			log.Printf("[SERVER][%s] role confirmed by RM: %v", s.ReplicaId, newRole)
		}
	}
}

func (s *server) Run() error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if s.CheckpointFreq > 0 {
		go func() {
			t := time.NewTicker(s.CheckpointFreq)
			defer t.Stop()
			for range t.C {
				s.dialBackups()
				s.sendCheckpoint()
			}
		}()
	}

	if s.rmAddr != "" {
		go s.connectRM()
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
