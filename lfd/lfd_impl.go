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

	// Try initial connection
	if l.conn == nil {
		if err := l.connect(); err != nil {
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
			os.Exit(0)
		}
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
			log.Printf("[LFD][%s] connect failed: %v; server appears to be down", l.replicaID, err)
			fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
			os.Exit(0)
		}
	}

	l.heartbeatCnt++

	// Send PING
	_ = l.conn.SetWriteDeadline(time.Now().Add(l.timeout))
	hb := ping
	if err := utils.WriteLine(l.conn, hb); err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT SEND FAILED to %s: %v  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
		os.Exit(0)
	}
	log.Printf("[%s] [heartbeat_count=%d] LFD->S send heartbeat: '%s'",
		l.lfdTag(), l.heartbeatCnt, hb)

	// Expect PONG
	_ = l.conn.SetReadDeadline(time.Now().Add(l.timeout))
	line, err := utils.ReadLine(l.reader)
	if err != nil {
		log.Printf("[%s] [heartbeat_count=%d] HEARTBEAT RECV FAILED from %s: %v  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt, l.serverAddr, err)
		fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
		os.Exit(0)
	}

	if line == pong {
		log.Printf("[%s] [heartbeat_count=%d] S->LFD recv heartbeat reply: '%s'",
			l.lfdTag(), l.heartbeatCnt, line)
	} else {
		log.Printf("[%s] [heartbeat_count=%d] UNEXPECTED REPLY '%s' (expected PONG)  <-- DETECTED CRASH",
			l.lfdTag(), l.heartbeatCnt, line)
		fmt.Printf("SERVER %s DOWN\n", l.serverAddr)
		os.Exit(0)
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
