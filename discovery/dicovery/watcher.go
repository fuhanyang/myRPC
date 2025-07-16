package dicovery

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// Watcher 与服务注册端建立联系，通过watch回调事件向endpoint更新节点信息
type Watcher struct {
	UpChan    chan Event
	conn      net.Conn
	once      sync.Once
	CloseChan chan struct{}
}

type Event struct {
	Type  eventType
	Key   string //原先监听的key
	Value []byte
}

type eventType string

const (
	Add    eventType = "add"
	Del    eventType = "del"
	Update eventType = "update"
)

//读服务端回调消息，解析消息内容，并更新节点信息
//通过协议确定消息长度防止粘包

func NewWatcher(ctx context.Context, conn net.Conn) *Watcher {
	w := &Watcher{}
	w.UpChan = make(chan Event, 10)
	w.CloseChan = make(chan struct{})
	w.conn = conn
	go func() {
		select {
		case <-ctx.Done():
		case <-w.CloseChan:
		}
		w.Close()
	}()
	go w.Watch()
	return w
}

func (w *Watcher) Watch() {
	var event Event
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in f", r)
		}
	}()
	for {
		buf := make([]byte, 4) // 假设消息长度前缀是 4 个字节（uint32）
		// 首先读取消息长度前缀
		_, err := io.ReadFull(w.conn, buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Client disconnected")
			} else {
				fmt.Println("Error reading length prefix:", err)
			}
			w.Close()
			return
		}
		// 将前缀转换为消息长度
		messageLength := binary.BigEndian.Uint32(buf)
		messageBuf := make([]byte, messageLength)
		// 读取完整的消息体
		_, err = io.ReadFull(w.conn, messageBuf)
		if err != nil {
			fmt.Println("Error reading message:", err)
			return
		}
		// 处理消息
		err = json.Unmarshal(messageBuf, &event)
		if err != nil {
			fmt.Println("Error unmarshalling message:", err)
		}
		fmt.Println("Received event:", event.Key, event.Type)
		w.UpChan <- event
	}

}
func (w *Watcher) Close() {
	w.once.Do(func() {
		fmt.Println("Closing watcher")
		err := w.conn.Close()
		if err != nil {
			log.Printf("close watcher error:%v", err)
		}

		close(w.UpChan)
		close(w.CloseChan)
	})

}
