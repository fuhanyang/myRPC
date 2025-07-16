package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"registry/registry"
	"server/server"
	"time"
)

func main() {

	r := registry.DefaultRegistry()
	err := r.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	r.Start(context.Background())
	for i := 0; i < 3; i++ {
		go startServer()
	}
	select {}
}

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}
func (f Foo) Multiple(args Args, reply *int) error {
	*reply = args.Num1 * args.Num2
	return nil
}
func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}
func startServer() {
	var foo Foo
	l, _ := net.Listen("tcp", ":0")
	fmt.Println("start server at", l.Addr().String())
	server := server.NewServer()
	_ = server.Register(&foo)
	// 向注册中心注册服务
	r := registry.DefaultRegistry()
	_, err := r.Register(context.Background(), "Foo", "tcp@"+l.Addr().String(), "tcp")
	if err != nil {
		log.Fatal(err)
	}
	server.Accept(l)
}
