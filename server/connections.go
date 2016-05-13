package server

import (
	"sync"

	"net"
)

type ConnectionManager struct {
	*sync.WaitGroup
	Conns   []*net.TCPConn
	Counter int
}

func NewConnectionManager() *ConnectionManager {
	cm := &ConnectionManager{}
	cm.WaitGroup = &sync.WaitGroup{}
	return cm
}

func (cm *ConnectionManager) Add(delta int) {
	cm.Counter += delta
	cm.WaitGroup.Add(delta)
}

func (cm *ConnectionManager) Done() {
	cm.Counter--
	cm.WaitGroup.Done()
}
