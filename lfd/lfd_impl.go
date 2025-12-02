package lfd

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wenyinh/18749-project/utils"
)

const (
	ping     = "PING"
	pong     = "PONG"
	register = "REGISTER"
	ack      = "ACK"
	nack     = "NACK"
	gfdPing  = "GFD_PING"
	gfdPong  = "GFD_PONG"
)

type LFDConfig struct {
	LFDID      string
	TargetAddr string
	GFDAddr    string
	HBFreq     time.Duration
	Timeout    time.Duration
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration

	ServerID       string
	ServerAddr     string
	Backups        string
	CkptMs         int
	StartAsNewborn bool
	InitState      int
}

type lfd struct {
	lfdID          string // LFD's own ID
	serverID       string // Server's ID that this LFD is monitoring
	serverAddr     string
	hbFreq         time.Duration
	timeout        time.Duration
	heartbeatCnt   int
	conn           net.Conn
	reader         *bufio.Reader
	gfdAddr        string
	gfdConn        net.Conn
	gfdReader      *bufio.Reader
	maxRetries     int
	baseDelay      time.Duration
	maxDelay       time.Duration
	firstHeartbeat bool

	serverListenAddr string
	backups          string
	ckptMs           int
	startAsNewborn   bool
	initState        int
	procMu           sync.Mutex
	serverProcess    *exec.Cmd
}

func getServerID(lfdID string) string {
	if lfdID[:3] != "LFD" {
		return ""
	}
	return "S" + lfdID[3:]
}

func NewLFD(lfdID, serverAddr, gfdAddr string, hbFreq, timeout time.Duration, maxRetries int, baseDelay, maxDelay time.Duration) LFD {
	cfg := LFDConfig{
		LFDID:      lfdID,
		TargetAddr: serverAddr,
		ServerAddr: serverAddr,
		GFDAddr:    gfdAddr,
		HBFreq:     hbFreq,
		Timeout:    timeout,
		MaxRetries: maxRetries,
		BaseDelay:  baseDelay,
		MaxDelay:   maxDelay,
	}
	return NewLFDWithConfig(cfg)
}

func NewLFDWithConfig(config LFDConfig) LFD {
	serverID := config.ServerID
	if serverID == "" {
		serverID = getServerID(config.LFDID)
	}
	listenAddr := config.ServerAddr
	if listenAddr == "" {
		listenAddr = config.TargetAddr
	}
	return &lfd{
		lfdID:            config.LFDID,
		serverID:         serverID,
		serverAddr:       config.TargetAddr,
		hbFreq:           config.HBFreq,
		timeout:          config.Timeout,
		gfdAddr:          config.GFDAddr,
		maxRetries:       config.MaxRetries,
		baseDelay:        config.BaseDelay,
		maxDelay:         config.MaxDelay,
		firstHeartbeat:   true,
		serverListenAddr: listenAddr,
		backups:          config.Backups,
		ckptMs:           config.CkptMs,
		startAsNewborn:   config.StartAsNewborn,
		initState:        config.InitState,
	}
}

func (l *lfd) Run() error {
	log.Printf("[LFD][%s] starting; monitoring server=%s at %s freq=%s timeout=%s",
		l.lfdID, l.serverID, l.serverAddr, l.hbFreq, l.timeout)

	// Connect to GFD first (GFD should be running)
	if err := l.connectToGFD(); err != nil {
		log.Printf("[LFD][%s] failed to connect to GFD at %s: %v", l.lfdID, l.gfdAddr, err)
		return err
	}

	log.Printf("[LFD][%s] registered with GFD, waiting for server %s to start...", l.lfdID, l.serverID)

	// Don't exit if server is not running yet - keep trying in heartbeat loop
	// Server will be started later by the user

	// Start heartbeat loop - will continuously try to connect to server
	t := time.NewTicker(l.hbFreq)
	defer t.Stop()

	for range t.C {
		l.sendOneHeartbeat()
	}

	return nil
}

func (l *lfd) sendOneHeartbeat() {
	if l.conn == nil {
		if err := l.connectWithRetry(); err != nil {
			if l.firstHeartbeat {
				log.Printf("[LFD][%s] server %s not available yet, waiting...", l.lfdID, l.serverID)
				return
			}
			log.Printf("[LFD][%s] connect failed after retries; server %s appears to be down", l.lfdID, l.serverID)
			l.handleServerDown("connect failed")
			return
		}
	}

	l.heartbeatCnt++
	cyan := "\033[36m"
	reset := "\033[0m"

	_ = l.conn.SetWriteDeadline(time.Now().Add(l.timeout))
	hb := ping
	if err := utils.WriteLine(l.conn, hb); err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT SEND FAILED to %s: %v",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		l.resetConn()
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
				l.lfdTag(), l.heartbeatCnt)
			l.handleServerDown("send failed")
		}
		return
	}
	log.Printf("%s[%s] [heartbeat_count=%d] LFD->S send heartbeat: '%s'%s",
		cyan, l.lfdTag(), l.heartbeatCnt, hb, reset)

	_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	line, err := utils.ReadLine(l.reader)
	if err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT RECV FAILED from server %s: %v",
			l.lfdTag(), l.heartbeatCnt, l.serverID, err)
		l.resetConn()
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
				l.lfdTag(), l.heartbeatCnt)
			l.handleServerDown("recv failed")
		}
		return
	}

	if line == pong {
		log.Printf("%s[%s] [heartbeat_count=%d] S->LFD recv heartbeat reply: '%s'%s",
			cyan, l.lfdTag(), l.heartbeatCnt, line, reset)
		if l.firstHeartbeat {
			l.firstHeartbeat = false
			l.notifyGFD("ADD")
		}
		return
	}

	log.Printf("[%s] [heartbeat_count=%d] UNEXPECTED REPLY '%s' (expected PONG)",
		l.lfdTag(), l.heartbeatCnt, line)
	l.resetConn()
	if err := l.connectWithRetry(); err != nil {
		log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt)
		l.handleServerDown("unexpected reply")
	}
}

func (l *lfd) connect() error {
	log.Printf("[LFD][%s] connecting to %s to monitor server %s ...", l.lfdID, l.serverAddr, l.serverID)
	conn, err := net.Dial("tcp", l.serverAddr)
	if err != nil {
		log.Printf("[LFD][%s] connection to %s failed: %v", l.lfdID, l.serverAddr, err)
		return err
	}
	l.conn = conn
	l.reader = bufio.NewReader(l.conn)

	// Send REGISTER handshake with server ID
	log.Printf("[LFD][%s] sending registration for server %s", l.lfdID, l.serverID)
	_ = l.conn.SetWriteDeadline(time.Now().Add(l.timeout))
	registerMsg := fmt.Sprintf("%s %s", register, l.serverID)
	if err := utils.WriteLine(l.conn, registerMsg); err != nil {
		log.Printf("[LFD][%s] failed to send registration: %v", l.lfdID, err)
		_ = l.conn.Close()
		l.conn = nil
		l.reader = nil
		return err
	}

	// Wait for ACK or NACK
	_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	response, err := utils.ReadLine(l.reader)
	if err != nil {
		log.Printf("[LFD][%s] failed to receive registration response: %v", l.lfdID, err)
		_ = l.conn.Close()
		l.conn = nil
		l.reader = nil
		return err
	}

	if response != ack {
		log.Printf("[LFD][%s] server rejected registration with response: %s", l.lfdID, response)
		_ = l.conn.Close()
		l.conn = nil
		l.reader = nil
		return fmt.Errorf("server rejected registration: expected server ID %s", l.serverID)
	}

	log.Printf("[LFD][%s] successfully registered to monitor server %s at %s", l.lfdID, l.serverID, l.serverAddr)
	return nil
}

func (l *lfd) connectWithRetry() error {
	for attempt := 0; attempt <= l.maxRetries; attempt++ {
		err := l.connect()
		if err == nil {
			return nil
		}

		if attempt == l.maxRetries {
			log.Printf("[LFD][%s] Failed to connect to server %s after %d attempts", l.lfdID, l.serverID, l.maxRetries+1)
			return err
		}

		delay := l.calculateBackoffDelay(attempt)
		log.Printf("[LFD][%s] Retry %d/%d: reconnecting to server %s in %v...", l.lfdID, attempt+1, l.maxRetries, l.serverID, delay)
		time.Sleep(delay)
	}
	return fmt.Errorf("max retries exceeded")
}

func (l *lfd) calculateBackoffDelay(attempt int) time.Duration {
	delay := time.Duration(1<<uint(attempt)) * l.baseDelay
	if delay > l.maxDelay {
		delay = l.maxDelay
	}
	return delay
}

func (l *lfd) connectToGFD() error {
	log.Printf("[LFD][%s] connecting to GFD at %s ...", l.lfdID, l.gfdAddr)
	conn, err := net.Dial("tcp", l.gfdAddr)
	if err != nil {
		log.Printf("[LFD][%s] failed to connect to GFD: %v", l.lfdID, err)
		return err
	}
	l.gfdConn = conn
	l.gfdReader = bufio.NewReader(conn)

	// Send REGISTER message to GFD
	registerMsg := fmt.Sprintf("REGISTER %s %s", l.serverID, l.lfdID)
	err = utils.WriteLine(l.gfdConn, registerMsg)
	if err != nil {
		log.Printf("[LFD][%s] failed to register with GFD: %v", l.lfdID, err)
		_ = l.gfdConn.Close()
		l.gfdConn = nil
		l.gfdReader = nil
		return err
	}

	log.Printf("[LFD][%s] registered with GFD to monitor server %s", l.lfdID, l.serverID)

	// Start goroutine to handle GFD heartbeats
	go l.handleGFDHeartbeats()

	return nil
}

func (l *lfd) handleGFDHeartbeats() {
	log.Printf("[LFD][%s] starting GFD heartbeat handler", l.lfdID)
	for {
		if l.gfdReader == nil || l.gfdConn == nil {
			log.Printf("[LFD][%s] GFD connection lost, stopping heartbeat handler", l.lfdID)
			return
		}

		// Read message from GFD (blocking)
		line, err := utils.ReadLine(l.gfdReader)
		if err != nil {
			log.Printf("[LFD][%s] GFD connection closed: %v", l.lfdID, err)
			return
		}

		// Handle GFD_PING
		if line == gfdPing {
			// Respond with GFD_PONG
			err := utils.WriteLine(l.gfdConn, gfdPong)
			if err != nil {
				log.Printf("[LFD][%s] failed to send GFD_PONG: %v", l.lfdID, err)
				return
			}
			log.Printf("[LFD][%s] responded to GFD heartbeat with GFD_PONG", l.lfdID)
		} else {
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			cmd := strings.ToUpper(parts[0])
			switch cmd {
			case "START":
				if len(parts) >= 2 {
					serverID := parts[1]
					if serverID == l.serverID {
						log.Printf("[LFD][%s] Received START command from GFD for server %s", l.lfdID, serverID)
						if err := l.startServer(); err != nil {
							log.Printf("[LFD][%s] Failed to start server: %v", l.lfdID, err)
						} else {
							l.firstHeartbeat = true
						}
					}
				}
			default:
				log.Printf("[LFD][%s] received unexpected message from GFD: %s", l.lfdID, line)
			}
		}
	}
}

func (l *lfd) notifyGFD(action string) {
	if l.gfdConn == nil {
		log.Printf("[LFD][%s] no GFD connection, skipping %s notification for server %s", l.lfdID, action, l.serverID)
		return
	}

	// Send server ID to GFD, not LFD ID
	msg := fmt.Sprintf("%s %s %s", action, l.serverID, l.lfdID)
	err := utils.WriteLine(l.gfdConn, msg)
	if err != nil {
		log.Printf("[LFD][%s] failed to send %s for server %s to GFD: %v", l.lfdID, action, l.serverID, err)
	} else {
		log.Printf("[LFD][%s] sent %s for server %s to GFD", l.lfdID, action, l.serverID)
	}
}

func (l *lfd) resetConn() {
	if l.conn != nil {
		_ = l.conn.Close()
	}
	l.conn = nil
	l.reader = nil
}

func (l *lfd) lfdTag() string {
	return fmt.Sprintf("LFD][%s->%s", l.lfdID, l.serverID)
}

func (l *lfd) handleServerDown(reason string) {
	l.notifyGFD("DELETE")
	fmt.Printf("SERVER %s DOWN\n", l.serverID)
	if l.conn != nil {
		_ = l.conn.Close()
	}
	l.conn = nil
	l.reader = nil
	l.firstHeartbeat = true
	log.Printf("[LFD][%s] server %s down (%s); waiting for RM to restart", l.lfdID, l.serverID, reason)
}

func (l *lfd) startServer() error {
	if l.serverListenAddr == "" {
		return fmt.Errorf("missing server listen address for %s", l.serverID)
	}

	l.procMu.Lock()
	if l.serverProcess != nil && l.serverProcess.ProcessState == nil {
		pid := l.serverProcess.Process.Pid
		l.procMu.Unlock()
		log.Printf("[LFD][%s] server process already running (PID %d)", l.lfdID, pid)
		return nil
	}
	l.procMu.Unlock()

	args := []string{
		"run",
		"cmd/server/srunner.go",
		"-rid", l.serverID,
		"-addr", l.serverListenAddr,
		"-init_state", strconv.Itoa(l.initState),
	}
	if l.backups != "" {
		args = append(args, "-backups", l.backups)
	}
	if l.ckptMs > 0 {
		args = append(args, "-ckpt_ms", strconv.Itoa(l.ckptMs))
	}
	if l.startAsNewborn {
		args = append(args, "-newborn")
	}

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server process: %w", err)
	}

	l.procMu.Lock()
	l.serverProcess = cmd
	l.procMu.Unlock()

	log.Printf("[LFD][%s] ✅ Server %s restarted with PID %d", l.lfdID, l.serverID, cmd.Process.Pid)

	go func(c *exec.Cmd, tag string) {
		err := c.Wait()
		if err != nil {
			log.Printf("[LFD][%s] server process exited: %v", tag, err)
		} else {
			log.Printf("[LFD][%s] server process exited normally", tag)
		}
		l.procMu.Lock()
		if l.serverProcess == c {
			l.serverProcess = nil
		}
		l.procMu.Unlock()
	}(cmd, l.lfdID)

	return nil
}
