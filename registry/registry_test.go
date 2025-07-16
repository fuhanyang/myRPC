package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"registry/registry/proxy"
	"registry/registry/storeEngine"
	"registry/registry/watcher"
	"strings"
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	//go func() {
	//	for i := 0; ; i++ {
	//		NewClient(i)
	//		time.Sleep(1 * time.Second)
	//	}
	//}()
	l, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	go ServerDo()
	store := storeEngine.GetTrigger()
	p := proxy.GetDefaultProxy()
	p.ConnectStore(store)
	var i int
	for {
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		//fmt.Println("client connected", conn.RemoteAddr().String())

		// 读取客户端名称（或唯一标识符）
		reader := bufio.NewReader(conn)
		clientName, _ := reader.ReadString('\n')
		clientName = strings.TrimSpace(clientName)

		split := strings.Split(clientName, "_")
		if len(split) != 2 {
			panic("client name should be in format 'id_target'")
		}
		fmt.Println("client name:", clientName)
		// 启动一个 watcher 来监听服务端的变化
		watcher := watcher.NewWatcher(conn, split[0])
		p.CreateProxy(watcher, split[1])

		//go func(i int) {
		//	time.Sleep(time.Second * 5)
		//	//fmt.Println("client", i, "exit", watcher.Uid)
		//	watcher.Close()
		//}(i)

		i++
		buffer := make([]byte, 1024)
		_, err = reader.Read(buffer)
		if err == io.EOF {
			fmt.Println("client disconnected", conn.RemoteAddr().String())
			watcher.Close()
		}
	}

}
func ServerDo() {
	t := storeEngine.GetTrigger()
	reply, err := t.Do(context.Background(), storeEngine.AddServiceCmdType, &storeEngine.Endpoint{ServiceName: "testService"})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(" add service reply:", string(reply.ToBytes()))
	var i int
	for {
		i++
		reply, err = t.Do(context.Background(),
			storeEngine.AddEndpointCmdType,
			&storeEngine.Endpoint{ServiceName: "testService", URL: fmt.Sprintf("localhost:8080/%d", i),
				Protocol: "tcp"})
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println(string(reply.ToBytes()))

		time.Sleep(time.Second * 5)
	}
}
func NewClient(i int) {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	_, err = conn.Write([]byte(fmt.Sprintf("%d_testService\n", i)))
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			time.Sleep(time.Second * 5)
			buf := make([]byte, 4) // 假设消息长度前缀是 4 个字节（uint32）
			// 首先读取消息长度前缀
			_, err := io.ReadFull(conn, buf)
			if err != nil {
				if err == io.EOF {
					fmt.Println("Client disconnected")
				} else {
					fmt.Println("Error reading length prefix:", err)
				}
				return
			}
			// 将前缀转换为消息长度
			messageLength := binary.BigEndian.Uint32(buf)
			messageBuf := make([]byte, messageLength)
			// 读取完整的消息体
			_, err = io.ReadFull(conn, messageBuf)
			if err != nil {
				fmt.Println("Error reading message:", err)
				return
			}
			// 处理消息
			fmt.Printf("Received event: %s\n", string(messageBuf))
		}
	}()
}
