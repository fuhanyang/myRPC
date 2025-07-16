package watcher

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type Watcher interface {
	Watch(ctx context.Context, target string)
	Close()
	Connect() chan []byte
	GetUid() string
	GetTarget() string
	IsAlive() bool
}

type DefaultWatcher struct {
	target      string
	conn        net.Conn
	Uid         string
	InChan      chan []byte
	OutChan     chan []byte
	CancelFunc  context.CancelFunc
	startTime   time.Time
	startTimeMu sync.RWMutex
	life        time.Duration
	once        sync.Once
}

func NewWatcher(conn net.Conn, uid string) *DefaultWatcher {
	w := &DefaultWatcher{
		conn:      conn,
		Uid:       uid,
		InChan:    make(chan []byte, 1024),
		OutChan:   make(chan []byte, 1024),
		life:      time.Second * 3,
		startTime: time.Now(),
	}
	return w
}
func (w *DefaultWatcher) GetUid() string {
	return w.Uid
}
func (w *DefaultWatcher) GetTarget() string {
	return w.target
}

// Watch 把InChan交给代理层，这样存储引擎的变化就能告知watcher
func (w *DefaultWatcher) Watch(ctx context.Context, target string) {
	w.target = target
	ctx, cancel := context.WithCancel(context.Background())
	w.CancelFunc = cancel
	go w.watch(ctx)
	go w.send(ctx)
	go w.heartbeat(ctx)
	go w.listen()
}
func (w *DefaultWatcher) listen() {
	reader := bufio.NewReader(w.conn)
	for {
		// 尝试从连接中读取数据
		buffer := make([]byte, 1024)
		_, err := reader.Read(buffer)
		if err == io.EOF {
			w.Close()
			return
		}
	}

}
func (w *DefaultWatcher) watch(ctx context.Context) {
	var data []byte
	for {
		select {
		case <-ctx.Done():
			close(w.InChan)
			return
		case data = <-w.InChan:
			w.OutChan <- data
		}
	}
}

func (w *DefaultWatcher) Connect() chan []byte {
	return w.InChan
}

// Send 收到回调事件的信息就将其发送给客户端
func (w *DefaultWatcher) send(ctx context.Context) {
	var data []byte
	var err error
	for {
		select {
		case data = <-w.OutChan:
			// 计算消息长度前缀
			prefixLen := uint32(len(data))
			// 创建一个缓冲区来存储前缀
			prefixBuf := make([]byte, 4) // 因为 uint32 是 4 字节
			binary.BigEndian.PutUint32(prefixBuf, prefixLen)

			// 发送前缀
			_, err = w.conn.Write(prefixBuf)
			if err != nil {
				// 处理写入错误
				fmt.Println("watcher send prefix error", err)
				return
			}

			// 发送消息体
			_, err = w.conn.Write(data)
			if err != nil {
				fmt.Println("watcher send message error", err)
				break
			}

		case <-ctx.Done():
			close(w.OutChan)
			return
		}
	}
}
func (w *DefaultWatcher) Close() {
	w.once.Do(func() {
		fmt.Println("Close watcher", w.Uid)
		w.CancelFunc()
		err := w.conn.Close()
		if err != nil {
			panic(err)
		}
		// 使watcher的生命周期过期
		w.startTime = time.Now().Add(-2 * w.life)
	})
}

// Heartbeat 心跳检测，这样如果watch被终止后一段时间后代理层就能对其进行垃圾回收
func (w *DefaultWatcher) heartbeat(ctx context.Context) {
	timer := time.NewTimer(w.life / 2)
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			timer.Reset(w.life / 2)
			w.setLife(time.Now())
		}
	}
}

func (w *DefaultWatcher) setLife(time time.Time) {
	w.startTimeMu.Lock()
	w.startTime = time
	w.startTimeMu.Unlock()
}
func (w *DefaultWatcher) IsAlive() bool {
	w.startTimeMu.RLock()
	defer w.startTimeMu.RUnlock()
	if w.startTime.Add(w.life).Before(time.Now()) {
		return false
	}
	return true
}
