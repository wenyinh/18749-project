package lfd

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
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
}

func getServerID(lfdID string) string {
	if lfdID[:3] != "LFD" {
		return ""
	}
	return "S" + lfdID[3:]
}

func NewLFD(lfdID, serverAddr, gfdAddr string, hbFreq, timeout time.Duration, maxRetries int, baseDelay, maxDelay time.Duration) LFD {
	return &lfd{
		lfdID:          lfdID,              // LFD's own ID
		serverID:       getServerID(lfdID), // Server ID to monitor
		serverAddr:     serverAddr,
		hbFreq:         hbFreq,
		timeout:        timeout,
		gfdAddr:        gfdAddr,
		maxRetries:     maxRetries,
		baseDelay:      baseDelay,
		maxDelay:       maxDelay,
		firstHeartbeat: true,
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
			// If we never had a successful connection (firstHeartbeat == true),
			// then server hasn't started yet - just keep waiting
			if l.firstHeartbeat {
				log.Printf("[LFD][%s] server %s not available yet, waiting...", l.lfdID, l.serverID)
				return
			}
			// If we HAD a connection before, this is a real failure
			log.Printf("[LFD][%s] connect failed after retries; server %s appears to be down", l.lfdID, l.serverID)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverID)
			os.Exit(0)
		}
	}

	l.heartbeatCnt++

	// Send PING
	_ = l.conn.SetWriteDeadline(time.Now().Add(l.timeout))
	hb := ping
	if err := utils.WriteLine(l.conn, hb); err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT SEND FAILED to %s: %v",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		l.resetConn()

		// Try to reconnect
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
				l.lfdTag(), l.heartbeatCnt)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverID)
			os.Exit(0)
		}
		return
	}
	cyan := "\033[36m"
	reset := "\033[0m"
	log.Printf("%s[%s] [heartbeat_count=%d] LFD->S send heartbeat: '%s'%s",
		cyan, l.lfdTag(), l.heartbeatCnt, hb, reset)

	// Expect PONG
	_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	line, err := utils.ReadLine(l.reader)
	if err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT RECV FAILED from server %s: %v",
			l.lfdTag(), l.heartbeatCnt, l.serverID, err)
		l.resetConn()

		// Try to reconnect
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
				l.lfdTag(), l.heartbeatCnt)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverID)
			os.Exit(0)
		}
		return
	}

	if line == pong {
		log.Printf("%s[%s] [heartbeat_count=%d] S->LFD recv heartbeat reply: '%s'%s",
			cyan, l.lfdTag(), l.heartbeatCnt, line, reset)

		// If this is the first successful heartbeat, notify GFD
		if l.firstHeartbeat {
			l.firstHeartbeat = false
			l.notifyGFD("ADD")
		}
	} else {
		log.Printf("[%s] [heartbeat_count=%d] UNEXPECTED REPLY '%s' (expected PONG)",
			l.lfdTag(), l.heartbeatCnt, line)
		l.resetConn()

		// Try to reconnect
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
				l.lfdTag(), l.heartbeatCnt)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverID)
			os.Exit(0)
		}
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
			log.Printf("[LFD][%s] received unexpected message from GFD: %s", l.lfdID, line)
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
