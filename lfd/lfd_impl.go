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
	ping = "PING"
	pong = "PONG"
)

type lfd struct {
	replicaID      string
	serverAddr     string
	hbFreq         time.Duration
	timeout        time.Duration
	heartbeatCnt   int
	conn           net.Conn
	reader         *bufio.Reader
	gfdAddr        string
	gfdConn        net.Conn
	maxRetries     int
	baseDelay      time.Duration
	maxDelay       time.Duration
	firstHeartbeat bool
}

func NewLFD(replicaID, serverAddr, gfdAddr string, hbFreq, timeout time.Duration, maxRetries int, baseDelay, maxDelay time.Duration) LFD {
	return &lfd{
		replicaID:      replicaID,
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
	log.Printf("[LFD][%s] starting; server=%s freq=%s timeout=%s",
		l.replicaID, l.serverAddr, l.hbFreq, l.timeout)

	// Connect to GFD
	if err := l.connectToGFD(); err != nil {
		log.Printf("[LFD][%s] failed to connect to GFD at %s: %v", l.replicaID, l.gfdAddr, err)
		return err
	}

	// Try initial connection to server
	if l.conn == nil {
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[LFD][%s] failed to connect to server after retries", l.replicaID)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
			os.Exit(0)
		}
	}

	// Send one immediate heartbeat
	l.sendOneHeartbeat()

	// Then continue periodically
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
			log.Printf("[LFD][%s] connect failed after retries; server appears to be down", l.replicaID)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
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
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
			os.Exit(0)
		}
		return
	}
	log.Printf("[%s] [heartbeat_count=%d] LFD->S send heartbeat: '%s'",
		l.lfdTag(), l.heartbeatCnt, hb)

	// Expect PONG
	_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	line, err := utils.ReadLine(l.reader)
	if err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT RECV FAILED from %s: %v",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		l.resetConn()

		// Try to reconnect
		if err := l.connectWithRetry(); err != nil {
			log.Printf("[%s] [heartbeat_count=%d] Reconnection failed after retries  <-- DETECTED CRASH",
				l.lfdTag(), l.heartbeatCnt)
			l.notifyGFD("DELETE")
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
			os.Exit(0)
		}
		return
	}

	if line == pong {
		log.Printf("[%s] [heartbeat_count=%d] S->LFD recv heartbeat reply: '%s'",
			l.lfdTag(), l.heartbeatCnt, line)

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
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
			os.Exit(0)
		}
	}
}

func (l *lfd) connect() error {
	log.Printf("[LFD][%s] connecting to %s ...", l.replicaID, l.serverAddr)
	conn, err := net.Dial("tcp", l.serverAddr)
	if err != nil {
		log.Printf("[LFD][%s] connection to %s failed: %v", l.replicaID, l.serverAddr, err)
		return err
	}
	l.conn = conn
	l.reader = bufio.NewReader(l.conn)
	log.Printf("[LFD][%s] connected to %s", l.replicaID, l.serverAddr)
	return nil
}

func (l *lfd) connectWithRetry() error {
	for attempt := 0; attempt <= l.maxRetries; attempt++ {
		err := l.connect()
		if err == nil {
			return nil
		}

		if attempt == l.maxRetries {
			log.Printf("[LFD][%s] Failed to connect after %d attempts", l.replicaID, l.maxRetries+1)
			return err
		}

		delay := l.calculateBackoffDelay(attempt)
		log.Printf("[LFD][%s] Retry %d/%d: reconnecting in %v...", l.replicaID, attempt+1, l.maxRetries, delay)
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
	log.Printf("[LFD][%s] connecting to GFD at %s ...", l.replicaID, l.gfdAddr)
	conn, err := net.Dial("tcp", l.gfdAddr)
	if err != nil {
		log.Printf("[LFD][%s] failed to connect to GFD: %v", l.replicaID, err)
		return err
	}
	l.gfdConn = conn
	log.Printf("[LFD][%s] connected to GFD", l.replicaID)
	return nil
}

func (l *lfd) notifyGFD(action string) {
	if l.gfdConn == nil {
		log.Printf("[LFD][%s] no GFD connection, skipping %s notification", l.replicaID, action)
		return
	}

	msg := fmt.Sprintf("%s %s", action, l.replicaID)
	err := utils.WriteLine(l.gfdConn, msg)
	if err != nil {
		log.Printf("[LFD][%s] failed to send %s to GFD: %v", l.replicaID, action, err)
	} else {
		log.Printf("[LFD][%s] sent %s %s to GFD", l.replicaID, action, l.replicaID)
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
	return fmt.Sprintf("LFD][%s", l.replicaID)
}
