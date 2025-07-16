package pool

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Pool struct {
	capacity     int
	running      int
	runMu        sync.RWMutex
	mu           myLock
	workers      *Workers
	Cond         *sync.Cond
	WorkerSource sync.Pool
	waiting      int
	waitMu       sync.RWMutex
	cancel       context.CancelFunc
}

type myLock uint32

const expiryDuration = time.Second * 20
const maxBackoff = 16

func (l *myLock) Lock() {
	backoff := 1
	for !atomic.CompareAndSwapUint32((*uint32)(l), 0, 1) {
		for i := 0; i < backoff; i++ {
			runtime.Gosched()
		}
		if backoff < maxBackoff {
			backoff <<= 1
		}
	}
}
func (l *myLock) Unlock() {
	atomic.StoreUint32((*uint32)(l), 0)
}
func newMyLock() myLock {
	return myLock(0)
}
func (p *Pool) AddRunning(i int) {
	p.runMu.Lock()
	p.running += i
	p.runMu.Unlock()
}
func (p *Pool) Running() int {
	p.runMu.RLock()
	defer p.runMu.RUnlock()
	return p.running
}
func (p *Pool) Waiting() int {
	p.waitMu.RLock()
	defer p.waitMu.RUnlock()
	return p.waiting
}
func (p *Pool) AddWaiting(i int) {
	p.waitMu.Lock()
	p.waiting += i
	p.waitMu.Unlock()
}
func NewPool(size int) *Pool {
	p := &Pool{
		capacity: size,
		mu:       newMyLock(),
	}
	p.WorkerSource.New = func() interface{} {
		return &GoWorker{
			Pool: p,
			Task: make(chan func(), WorkerChanCap),
		}
	}
	p.workers = NewWorkers(size)
	p.Cond = sync.NewCond(&p.mu)
	var ctx context.Context
	ctx, p.cancel = context.WithCancel(context.Background())
	go p.purgePeriodically(ctx)
	return p
}
func (p *Pool) Submit(task func()) error {
	var w *GoWorker
	w = p.getUsefulWorker()
	if w == nil {
		return errors.New("worker pool is full")
	}
	w.Task <- task
	return nil
}
func (p *Pool) purgePeriodically(ctx context.Context) {
	heartbeat := time.NewTicker(expiryDuration)
	defer func() {
		heartbeat.Stop()
	}()
	for {
		select {
		case <-heartbeat.C:
		case <-ctx.Done():
			return
		}
		p.mu.Lock()
		expiredWorkers := p.workers.RetrieveExpiry(expiryDuration)
		p.mu.Unlock()

		for i := range expiredWorkers {
			expiredWorkers[i].Task <- nil
			expiredWorkers[i] = nil
		}
		if p.Running() == 0 || (p.Waiting() > 0 && p.Running() < p.capacity) {
			p.Cond.Broadcast()
		}
	}
}
func (p *Pool) getUsefulWorker() *GoWorker {
	if p == nil {
		fmt.Println("pool is nil")
		return nil
	}
	var w *GoWorker
	spawnWorker := func() {
		w = p.WorkerSource.Get().(*GoWorker)
		w.Run()
	}
	p.mu.Lock()
	w = p.workers.Detach()
	if w != nil {
		p.mu.Unlock()
	} else if capacity := p.capacity; capacity == -1 || capacity > p.Running() {
		p.mu.Unlock()
		spawnWorker()
	} else {
	retry:
		p.AddWaiting(1)
		p.Cond.Wait()
		p.AddWaiting(-1)
		var nw int
		nw = p.Running()
		if nw == 0 {
			p.mu.Unlock()
			spawnWorker()
			return w
		}
		if w = p.workers.Detach(); w == nil {
			if nw < p.capacity {
				p.mu.Unlock()
				spawnWorker()
				return w
			}
			goto retry
		}
		p.mu.Unlock()
	}
	return w
}
func (p *Pool) RevertWorker(w *GoWorker) bool {
	w.RecycleTime = time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.workers.Insert(w)
	if err != nil {
		return false
	}
	p.Cond.Signal()
	return true
}
