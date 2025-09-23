package client

type Client interface {
	Connect() error
	SendMessage(message string)
	Close()
}
