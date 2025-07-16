package storeEngine

import (
	"context"
	"sync"
	"time"
)

type StoreEngine interface {
	Close()
	Do(ctx context.Context, cmd string, endpoint *Endpoint) (Reply, error)
	Connect(ctx context.Context, target string) chan []byte
}

type Reply interface {
	ToBytes() []byte
}

type Service struct {
	Name      string
	Endpoints map[string]*Endpoint
	Ttl       time.Duration
	timeMu    sync.RWMutex
	StartTime time.Time
}

func (s *Service) HeartBeat() {
	s.timeMu.Lock()
	defer s.timeMu.Unlock()
	s.StartTime = time.Now()
}

func (s *Service) GetEndpoints() []Endpoint {
	var endpoints []Endpoint
	for _, endpoint := range s.Endpoints {
		endpoints = append(endpoints, *endpoint)
	}
	return nil
}

type Endpoint struct {
	ServiceName string
	URL         string
	Protocol    string
	startTime   time.Time
	timeMutex   sync.RWMutex
	owner       *Service
}

func (e *Endpoint) HeartBeat() {
	e.timeMutex.Lock()
	e.startTime = time.Now()
	e.timeMutex.Unlock()

	e.owner.HeartBeat()
}
func (e *Endpoint) GetStartTime() time.Time {
	e.timeMutex.RLock()
	defer e.timeMutex.RUnlock()
	return e.startTime
}
