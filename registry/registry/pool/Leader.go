package pool

import (
	"fmt"
	"github.com/spaolacci/murmur3"
	"sync"
)

var al *allLeaders
var mux sync.Mutex

const (
	MaxLeaderNum = 10
	Size         = 100000 //最大协程数
	HashLength   = 1000
)

type allLeaders struct {
	leaders []*leader
}

type AllLeaders interface {
	GetLeader(index int) *leader
	TaskToLeader(func()) error
}

func newAllLeaders() *allLeaders {
	leaders := make([]*leader, 0, MaxLeaderNum)
	for i := 0; i < MaxLeaderNum; i++ {
		leaders = append(leaders, newLeader(i))
	}
	return &allLeaders{leaders: leaders}
}
func (al *allLeaders) TaskToLeader(task func()) error {
	key := fmt.Sprintf("%p", &task)
	leaderIndex := al.hash(key) * MaxLeaderNum / HashLength
	err := al.GetLeader(leaderIndex).Pool.Submit(task)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}
func (al *allLeaders) hash(key string) int {
	return int(murmur3.Sum32([]byte(key))) % HashLength
}

type leader struct {
	Pool  *Pool
	Index int
}

func (al *allLeaders) GetLeader(index int) *leader {
	if len(al.leaders) <= index {
		return al.leaders[len(al.leaders)-1]
	}
	return al.leaders[index]
}
func newLeader(index int) *leader {
	return &leader{Pool: NewPool(Size), Index: index}
}
func (l *leader) GetLeaderRunning() int {
	return l.Pool.Running()
}
func GetAllLeaders() AllLeaders {
	if al != nil {
		return al
	}
	mux.Lock()
	defer mux.Unlock()
	if al != nil {
		return al
	}
	al = newAllLeaders()
	return al
}
