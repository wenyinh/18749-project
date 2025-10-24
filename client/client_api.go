package client

type Client interface {
	Connect() error
	SendMessage(message string)
	Close()
}

func NewClient(clientID string, serverAddrs map[string]string) Client
