package test

import (
	"bufio"
	"discovery/dicovery"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestDiscovery(t *testing.T) {
	addr := "127.0.0.1:8080"
	//var (
	//	event     = dicovery.Event{}
	//	endpoint  = storeEngine.Endpoint{}
	//	prefixLen uint32
	//)
	//go startServer()
	em := dicovery.NewEndpointManager(addr, "testService")
	defer func() {
		fmt.Println("Canceling endpoint manager")
		time.Sleep(time.Second)
		em.Cancel()
	}()

	em.Listen()
}
func startServer() {
	var (
		event     = dicovery.Event{}
		endpoint  = dicovery.Endpoint{ServiceName: "testService", URL: "127.0.0.1:8080"}
		prefixLen uint32
	)
	l, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	conn, err := l.Accept()

	if err != nil {
		panic(err)
	}
	fmt.Println("Accepted connection from", conn.RemoteAddr())
	uid, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		panic(err)
	}
	fmt.Println("Received Uid:", uid)

	// 假设 event 是一个要发送的结构体，已经被序列化为 JSON 字节数组 messageBytes
	event.Key = endpoint.URL
	event.Value, err = json.Marshal(endpoint)
	if err != nil {
		panic(err)
	}
	event.Type = dicovery.Add
	messageBytes, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}

	// 计算消息长度前缀
	prefixLen = uint32(len(messageBytes))

	// 创建一个缓冲区来存储前缀
	prefixBuf := make([]byte, 4) // 因为 uint32 是 4 字节
	binary.BigEndian.PutUint32(prefixBuf, prefixLen)

	// 发送前缀
	_, err = conn.Write(prefixBuf)
	if err != nil {
		// 处理写入错误
		return
	}

	// 发送消息体
	_, err = conn.Write(messageBytes)
	if err != nil {
		// 处理写入错误
		return
	}

}
