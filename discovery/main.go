package main

import (
	"discovery/dicovery"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Resource struct {
	em *dicovery.EndpointManager
}

// Close 关闭资源的方法
func (r *Resource) Close() {
	fmt.Println("Resource closed")
	fmt.Println("Canceling endpoint manager")
	r.em.Cancel()
	time.Sleep(time.Second)
}
func main() {
	addr := "127.0.0.1:8080"
	//var (
	//	event     = dicovery.Event{}
	//	endpoint  = storeEngine.Endpoint{}
	//	prefixLen uint32
	//)
	//go startServer()
	em := dicovery.NewEndpointManager(addr, "testService")
	// 设置一个通道来接收中断信号
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM) // 监听SIGINT和SIGTERM信号

	// 启动一个goroutine来处理信号
	go func() {
		var resource Resource
		resource.em = em
		sig := <-interrupt // 阻塞直到接收到信号
		fmt.Println()      // 换行，为了美观，在实际应用中可能不需要
		fmt.Println("Interrupt signal received:", sig)

		// 在这里执行资源关闭操作
		resource.Close()

	}()

	go em.Listen()
	time.Sleep(time.Second * 10)
}
