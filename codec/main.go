package main

import (
	"codec/codec"
	"codec/header"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"server/server"
	"time"
)

func startServer(addr chan string) {
	// pick a free port
	l, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start server server on", l.Addr())
	addr <- l.Addr().String()
	rpc.Accept(l)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)
	// send options
	_ = json.NewEncoder(conn).Encode(server.DefaultOption)
	cc := codec.NewGobCodec(conn)
	// send request & receive response
	for i := 0; i < 5; i++ {
		h := &header.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           int64(uint64(i)),
		}
		_ = cc.Write(h, fmt.Sprintf("server req %d", h.Seq))
		var readHeader header.Header
		_ = cc.ReadHeader(&readHeader)
		fmt.Println(readHeader.ServiceMethod, readHeader.Seq, "server req")
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}
