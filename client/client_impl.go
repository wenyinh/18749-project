package client

import (
	"bufio"
	"log"
	"net"
	"strconv"

	"github.com/wenyinh/18749-project/utils"
)

type client struct {
	clientID   int
	serverAddr string
	conn       net.Conn
	reqCounter int
}

func NewClient(clientID int, serverAddr string) Client {
	return &client{
		clientID:   clientID,
		serverAddr: serverAddr,
		reqCounter: 0,
	}
}

func (c *client) Connect() error {
	// If MustDial returns only net.Conn (no error), use: c.conn = utils.MustDial(c.serverAddr); return nil
	c.conn = utils.MustDial(c.serverAddr)
	log.Printf("[C%d] Connected to server at %s\n", c.clientID, c.serverAddr)
	return nil
}

func (c *client) SendMessage(payload string) {
	if c.conn == nil {
		log.Printf("[C%d] Not connected to server\n", c.clientID)
		return
	}

	// Build protocol message: REQ <client_id> <req_id>
	c.reqCounter++
	reqID := strconv.Itoa(c.reqCounter)
	msg := "REQ " + "C" + strconv.Itoa(c.clientID) + " " + reqID

	log.Printf("[C%d] C->S send request: <%s>; payload='%s'\n", c.clientID, msg, payload)

	// Send request line first; if you want to send payload too, you must agree the format with server_impl.
	if err := utils.WriteLine(c.conn, msg); err != nil {
		log.Printf("[C%d] Error sending request: %v\n", c.clientID, err)
		return
	}

	// Read exactly one line reply
	br := bufio.NewReader(c.conn)
	reply, err := utils.ReadLine(br)
	if err != nil {
		log.Printf("[C%d] Error receiving reply: %v\n", c.clientID, err)
		return
	}

	// Expect: RESP <client_id> <req_id> <state> [optionally add S1 in server reply later]
	log.Printf("[C%d] S->C recv reply: %s\n", c.clientID, reply)
}

func (c *client) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
		log.Printf("[C%d] Disconnected from server at %s\n", c.clientID, c.serverAddr)
	}
}
