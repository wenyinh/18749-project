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

type MessageType struct {
	Type string `json:"type"`
}

type server struct {
	Addr      string
	ReplicaId string

	mu           sync.Mutex
	ServerState  int
	CheckpointNo int

	iAmReady  bool
	highWater map[string]int // clientID -> max request_num

	Backups        map[string]string
	BackupConns    map[string]net.Conn
	CheckpointFreq time.Duration
}

func NewServer(
	addr, replicaId string,
	initState int,
	newborn bool,
	backups map[string]string,
	ckptFreq time.Duration,
) Server {
	if backups == nil {
		backups = make(map[string]string)
	}
	return &server{
		Addr:           addr,
		ReplicaId:      replicaId,
		ServerState:    initState,
		CheckpointNo:   0,
		iAmReady:       !newborn,
		highWater:      make(map[string]int),
		Backups:        backups,
		BackupConns:    make(map[string]net.Conn),
		CheckpointFreq: ckptFreq,
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
					if err := utils.WriteLine(conn, Ack); err == nil {
						isLFDConnection = true
						log.Printf("[SERVER][%s] LFD registered successfully", s.ReplicaId)
					}
				} else {
					_ = utils.WriteLine(conn, Nack)
					log.Printf("[SERVER][%s] rejected LFD registration: expected %s but got %s",
						s.ReplicaId, s.ReplicaId, requestedServerID)
					return
				}
			}
			continue
		}

		if line == Ping {
			if err := utils.WriteLine(conn, Pong); err == nil {
				log.Printf("[SERVER][%s] heartbeat, sent pong to LFD", s.ReplicaId)
			}
			continue
		}

		var mt MessageType
		if err := json.Unmarshal([]byte(line), &mt); err != nil {
			log.Printf("[SERVER][%s] failed to parse JSON: %v, line=%q", s.ReplicaId, err, line)
			_ = utils.WriteLine(conn, "ERROR: invalid JSON format")
			continue
		}

		switch mt.Type {
		case Req:
			var reqMsg RequestMessage
			if err := json.Unmarshal([]byte(line), &reqMsg); err != nil {
				log.Printf("[SERVER][%s] failed to parse REQ JSON: %v", s.ReplicaId, err)
				_ = utils.WriteLine(conn, "ERROR: invalid JSON format")
				continue
			}
			s.handleReq(conn, reqMsg)

		case Checkpoint:
			var ckpt CheckpointMessage
			if err := json.Unmarshal([]byte(line), &ckpt); err != nil {
				log.Printf("[SERVER][%s] bad CHECKPOINT json: %v", s.ReplicaId, err)
				continue
			}
			s.handleCheckpoint(ckpt)

		default:
			_ = utils.WriteLine(conn, "ERROR: unknown request type")
		}
	}
}

func (s *server) handleReq(conn net.Conn, reqMsg RequestMessage) {
	s.mu.Lock()
	if !s.iAmReady {
		prev := s.highWater[reqMsg.ClientID]
		if reqMsg.RequestNum > prev {
			s.highWater[reqMsg.ClientID] = reqMsg.RequestNum
		}
		cur := s.highWater[reqMsg.ClientID]
		s.mu.Unlock()

		log.Printf("[SERVER][%s] NEWBORN, log only: client=%s req=%d (highWater=%d)",
			s.ReplicaId, reqMsg.ClientID, reqMsg.RequestNum, cur)
		return
	}

	before := s.ServerState
	s.ServerState++
	after := s.ServerState
	s.mu.Unlock()

	log.Printf("[SERVER][%s] received REQ from client=%s, request_num=%d, msg=%s",
		s.ReplicaId, reqMsg.ClientID, reqMsg.RequestNum, reqMsg.Message)
	log.Printf("[SERVER][%s] server state before: %d, after: %d",
		s.ReplicaId, before, after)

	respMsg := ResponseMessage{
		Type:        Resp,
		ServerID:    s.ReplicaId,
		ClientID:    reqMsg.ClientID,
		RequestNum:  reqMsg.RequestNum,
		ServerState: after,
		Message:     reqMsg.Message,
	}

	jsonResp, err := json.Marshal(respMsg)
	if err != nil {
		log.Printf("[SERVER][%s] error marshaling response: %v", s.ReplicaId, err)
		_ = utils.WriteLine(conn, "ERROR: failed to create response")
		return
	}

	if err := utils.WriteLine(conn, string(jsonResp)); err == nil {
		log.Printf("[SERVER][%s] sent RESP to client=%s, request_num=%d, state=%d",
			s.ReplicaId, reqMsg.ClientID, reqMsg.RequestNum, after)
	}
}

func (s *server) handleCheckpoint(ckpt CheckpointMessage) {
	s.mu.Lock()
	if ckpt.CheckpointNum <= s.CheckpointNo {
		oldNo := s.CheckpointNo
		s.mu.Unlock()
		log.Printf("[SERVER][%s] ignore stale checkpoint from %s: recv_no=%d <= local_no=%d",
			s.ReplicaId, ckpt.ReplicaId, ckpt.CheckpointNum, oldNo)
		return
	}
	oldState := s.ServerState
	s.ServerState = ckpt.ServerState
	s.CheckpointNo = ckpt.CheckpointNum
	if !s.iAmReady {
		s.iAmReady = true
	}
	newState := s.ServerState
	newNo := s.CheckpointNo
	s.mu.Unlock()

	log.Printf("\033[34m[SERVER][%s] apply checkpoint from %s: state %d -> %d, ckpt_no=%d, READY=%v\033[0m",
		s.ReplicaId, ckpt.ReplicaId, oldState, newState, newNo, true)
}

// 建立到 backups 的 secondary channel
func (s *server) dialBackups() {
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
			log.Printf("[SERVER][%s] dial backup %s@%s failed: %v",
				s.ReplicaId, t.id, t.addr, err)
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

// 周期性广播 checkpoint 给其他 active replica
func (s *server) sendCheckpoint() {
	if s.CheckpointFreq <= 0 {
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
			log.Printf("[SERVER][%s] send checkpoint to %s failed: %v (drop conn)",
				s.ReplicaId, bid, err)
			s.mu.Lock()
			if old, ok := s.BackupConns[bid]; ok {
				_ = old.Close()
				delete(s.BackupConns, bid)
			}
			s.mu.Unlock()
		} else {
			log.Printf("\033[34m[SERVER][%s] checkpoint #%d sent to %s (state=%d)\033[0m",
				s.ReplicaId, ckpt.CheckpointNum, bid, ckpt.ServerState)
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
