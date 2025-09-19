package server

type Server interface {
	Run(addr, replicaId string) error
}
