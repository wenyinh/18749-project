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
	fmt.Printf("[C%d] Connected to server S1\n", c.clientID)
	return nil
}

func (c *client) SendMessage(message string) {
	if c.conn == nil {
		fmt.Printf("[C%d] Not connected to server\n", c.clientID)
		return
	}
	fmt.Printf("[C%d] Sending request: %s\n", c.clientID, message)
	err := utils.WriteLine(c.conn, message)
	if err != nil {
		fmt.Printf("[C%d] Error sending message: %v\n", c.clientID, err)
		return
	}

	// Receive and print reply
	buffer := make([]byte, 1024)
	n, err := c.conn.Read(buffer)
	if err != nil {
		fmt.Printf("[C%d] Error receiving reply: %v\n", c.clientID, err)
		return
	}
	reply := string(buffer[:n])
	fmt.Printf("[C%d] Received reply: %s\n", c.clientID, reply)
}

func (c *client) Close() {
	if c.conn != nil {
		c.conn.Close()
		fmt.Printf("[C%d] Disconnected from server S1\n", c.clientID)
	}
}
