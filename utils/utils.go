package utils

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

func WriteLine(conn net.Conn, s string) error {
	_, err := conn.Write([]byte(s + "\n"))
	return err
}

func ReadLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func MustListen(addr string) net.Listener {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(fmt.Errorf("listen %s failed: %w", addr, err))
	}
	return ln
}

func MustDial(addr string) (net.Conn) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		panic(fmt.Errorf("dial %s failed: %w", addr, err))
	}
	return c
}
