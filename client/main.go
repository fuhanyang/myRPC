package main

import (
	client2 "client/client"
	"context"
	"discovery/dicovery"
	"log"
)

type Foo int

type Args struct{ Num1, Num2 int }

func call(registry string) {
	//d := dicovery.NewGeeRegistryDiscovery(registry, 0)
	d := dicovery.GetMyRPCDiscovery()
	d.Discover(registry)
	xc := client2.NewXClient(d, client2.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	// send request & receive response
	for i := 0; i < 30; i++ {
		go func(i int) {
			foo(xc, context.Background(), "call", "Foo.Sum", &Args{Num1: i, Num2: i * i})
			//foo(xc, context.Background(), "call", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
			//foo(xc, context.Background(), "call", "Foo.Multiple", &Args{Num1: i, Num2: i * i})
		}(i)
	}

}
func foo(xc *client2.XClient, ctx context.Context, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
		//case "broadcast":
		//	err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success with args %d %d: reply = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func main() {
	log.SetFlags(0)
	registryAddr := "localhost:8080"
	go func() {
		call(registryAddr)
		//time.Sleep(10 * time.Millisecond)

	}()
	select {}
}
