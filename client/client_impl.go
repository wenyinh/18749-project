package client

import (
	"fmt"
	"github.com/wenyinh/18749-project/utils"
	"net"
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
	conn, err := utils.MustDial(c.serverAddr)
	if err != nil {
		return err
	}
	c.conn = conn
	fmt.Printf("[C%d] Connected to server S1\n", c.clientID)
	return nil
}

func (c *client) SendMessage(message string) {
	if c.conn == nil {
		log.Printf("[C%d] Not connected to server\n", c.clientID)
		return
	}
	fmt.Printf("[C%d] Sending request: %s\n", c.clientID, message)
	err := utils.WriteLine(c.conn, message)
	if err != nil {
		fmt.Printf("[C%d] Error sending message: %v\n", c.clientID, err)
		return
	}

	// Read exactly one line reply
	br := bufio.NewReader(c.conn)
	reply, err := utils.ReadLine(br)
	if err != nil {
		log.Printf("[C%d] Error receiving reply: %v\n", c.clientID, err)
		return
	}
	reply := string(buffer[:n])
	fmt.Printf("[C%d] Received reply: %s\n", c.clientID, reply)
}

func (c *client) Close() {
	if c.conn != nil {
		c.conn.Close()
		log.Printf("[C%d] Disconnected from server S1\n", c.clientID)
	}
}
