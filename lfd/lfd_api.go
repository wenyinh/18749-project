package lfd

import "time"

type LFD interface {
	Run() error
}

func NewLFD(replicaID, serverAddr, gfdAddr string, hbFreq, timeout time.Duration, maxRetries int, baseDelay, maxDelay time.Duration) LFD
