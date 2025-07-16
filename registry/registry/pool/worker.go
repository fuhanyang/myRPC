package pool

import (
	"time"
)

var WorkerChanCap = 100

type Workers struct {
	items  []*GoWorker
	expiry []*GoWorker
}
type GoWorker struct {
	Pool        *Pool
	Task        chan func()
	RecycleTime time.Time
}

func NewWorkers(size int) *Workers {
	return &Workers{
		items: make([]*GoWorker, 0, size),
	}
}
func (w *Workers) len() int {
	return len(w.items)
}
func (w *Workers) isEmpty() bool {
	return len(w.items) == 0
}
func (w *Workers) Insert(worker *GoWorker) error {
	w.items = append(w.items, worker)
	return nil
}
func (w *Workers) Detach() *GoWorker {
	l := len(w.items)
	if l == 0 {
		return nil
	}
	worker := w.items[l-1]
	w.items[l-1] = nil
	w.items = w.items[:l-1]
	return worker
}
func (w *Workers) RetrieveExpiry(duration time.Duration) []*GoWorker {
	n := w.len()
	if n == 0 {
		return nil
	}
	expiry := time.Now().Add(duration)
	index := w.binarySearch(0, n-1, expiry)

	w.expiry = w.expiry[:0]
	if index != -1 {
		w.expiry = append(w.expiry, w.items[:index+1]...)
		m := copy(w.items, w.items[index+1:])
		for i := m; i < n; i++ {
			w.items[i] = nil
		}
		w.items = w.items[:m]
	}
	return w.expiry
}
func (w *Workers) binarySearch(l, r int, duration time.Time) int {
	var mid int
	for l <= r {
		mid = (l + r) / 2
		if duration.Before(w.items[mid].RecycleTime) {
			r = mid + 1
		} else {
			l = mid + 1
		}
	}
	return mid
}
func (wg *GoWorker) Run() {
	wg.Pool.AddRunning(1)
	go func() {
		defer func() {
			wg.Pool.AddRunning(-1)
			wg.Pool.WorkerSource.Put(wg)
			wg.Pool.Cond.Signal()
		}()

		for f := range wg.Task {
			if f == nil {
				return
			}
			f()
			ok := wg.Pool.RevertWorker(wg)
			if !ok {
				return
			}

		}
	}()
}
