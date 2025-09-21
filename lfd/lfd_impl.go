package lfd

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/wenyinh/18749-project/utils"
)

const (
	ping = "PING"
	pong = "PONG"
)

type lfd struct {
	replicaID    string
	serverAddr   string
	hbFreq       time.Duration
	timeout      time.Duration
	heartbeatCnt int
	conn         net.Conn
	reader       *bufio.Reader
}

func NewLFD(replicaID, serverAddr string, hbFreq, timeout time.Duration) LFD {
	return &lfd{
		replicaID:  replicaID,
		serverAddr: serverAddr,
		hbFreq:     hbFreq,
		timeout:    timeout,
	}
}

func (l *lfd) Run() error {
	log.Printf("[LFD][%s] starting; server=%s freq=%s timeout=%s",
		l.replicaID, l.serverAddr, l.hbFreq, l.timeout)

	// Ensure connection first (MustDial will panic on failure)
	if l.conn == nil {
		_ = l.connect()
	}

	// Optional: send one immediate heartbeat for quicker feedback
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
		if err := l.connect(); err != nil {
			log.Printf("[LFD][%s] connect failed: %v; retrying in 1s", l.replicaID, err)
			time.Sleep(1 * time.Second)
			return
		}
	}

	l.heartbeatCnt++

	// Send PING
	_ = l.conn.SetWriteDeadline(time.Now().Add(l.timeout))
	hb := ping
	if err := utils.WriteLine(l.conn, hb); err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT SEND FAILED to %s: %v  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		l.resetConn()
		return
	}
	log.Printf("[%s] [heartbeat_count=%d] LFD->S send heartbeat: '%s'",
		l.lfdTag(), l.heartbeatCnt, hb)

	// Expect PONG
	_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	line, err := utils.ReadLine(l.reader)
	if err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT RECV FAILED from %s: %v  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		l.resetConn()
		return
	}

	if line == pong {
		log.Printf("[%s] [heartbeat_count=%d] S->LFD recv heartbeat reply: '%s'",
			l.lfdTag(), l.heartbeatCnt, line)
	} else {
		log.Printf("[%s] [heartbeat_count=%d] UNEXPECTED REPLY '%s' (expected PONG)  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt, line)
		l.resetConn()
	}
}

func (l *lfd) connect() error {
	log.Printf("[LFD][%s] connecting to %s ...", l.replicaID, l.serverAddr)
	conn := utils.MustDial(l.serverAddr) // panics on failure
	l.conn = conn
	l.reader = bufio.NewReader(l.conn)
	log.Printf("[LFD][%s] connected to %s", l.replicaID, l.serverAddr)
	return nil
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
