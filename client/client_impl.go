package client

import (
	"github.com/wenyinh/18749-project/utils"
	"log"
	"net"
	"strconv"
)

type client struct {
	clientID   int
	serverAddr string
	conn       net.Conn
	reqID      int
}

func NewClient(clientID int, serverAddr string) Client {
	return &client{
		clientID:   clientID,
		serverAddr: serverAddr,
	}
}

func (c *client) Connect() error {
	conn := utils.MustDial(c.serverAddr)
	c.conn = conn
	log.Printf("[C%d] Connected to server S1\n", c.clientID)
	return nil
}

func (c *client) SendMessage() {
	if c.conn == nil {
		log.Printf("[C%d] Not connected to server\n", c.clientID)
		return
	}
	c.reqID++
	fullMsg := "REQ C" + strconv.Itoa(c.clientID) + " " + strconv.Itoa(c.reqID)

	log.Printf("[C%d] Sending request: %s\n", c.clientID, fullMsg)
	err := utils.WriteLine(c.conn, fullMsg)
	if err != nil {
		log.Printf("[C%d] Error sending message: %v\n", c.clientID, err)
		return
	}

	// Receive and print reply
	buffer := make([]byte, 1024)
	n, err := c.conn.Read(buffer)
	if err != nil {
		log.Printf("[C%d] Error receiving reply: %v\n", c.clientID, err)
		return
	}
	reply := string(buffer[:n])
	log.Printf("[C%d] Received reply: %s\n", c.clientID, reply)
}

func (c *client) Close() {
	if c.conn != nil {
		c.conn.Close()
		log.Printf("[C%d] Disconnected from server S1\n", c.clientID)
	}
}
