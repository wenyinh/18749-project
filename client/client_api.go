package client

type Client interface {
	Connect() error
	SendMessage()
	Close()
}
