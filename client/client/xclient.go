package client

import (
	"context"
	"discovery/dicovery"
	"fmt"
	"io"
	"server/server"
	"strings"
	"sync"
)

type SelectMode int

const (
	RandomSelect     SelectMode = iota // select randomly
	RoundRobinSelect                   // select using Robbin algorithm

)

type XClient struct {
	d       *dicovery.MyRPCDiscovery
	mode    SelectMode
	opt     *server.Option
	mu      sync.Mutex // protect following
	clients map[string]*Client
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d *dicovery.MyRPCDiscovery, mode SelectMode, opt *server.Option) *XClient {
	return &XClient{d: d, mode: mode, opt: opt, clients: make(map[string]*Client)}
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		// I have no idea how to deal with error, just ignore it.
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}
func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, rpcAddr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = XDial(rpcAddr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	fmt.Println("call to server:", rpcAddr)
	client, err := xc.dial(rpcAddr)
	if err != nil {
		fmt.Println("dial error:", err)
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
// xc will choose a proper server.
func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	var endpoint = &dicovery.Endpoint{}
	split := strings.Split(serviceMethod, ".")
	endpoint = xc.d.Get(split[0])
	if endpoint == nil {
		return fmt.Errorf("endpoint not found: %s", serviceMethod)
	}

	return xc.call(endpoint.URL, ctx, serviceMethod, args, reply)
}

//// Broadcast invokes the named function for every server registered in discovery
//func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
//	servers, err := xc.d.GetAll()
//	if err != nil {
//		return err
//	}
//	var wg sync.WaitGroup
//	var mu sync.Mutex // protect e and replyDone
//	var e error
//	replyDone := reply == nil // if reply is nil, don't need to set value
//	ctx, cancel := context.WithCancel(ctx)
//	for _, rpcAddr := range servers {
//		fmt.Println("sending to server:", rpcAddr)
//		wg.Add(1)
//		//这里如果不赋值进去会因为循环闭包捕获导致值不可预测
//		go func(rpcAddr string) {
//			defer wg.Done()
//			var clonedReply interface{}
//			if reply != nil {
//				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
//			}
//			err := xc.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
//			mu.Lock()
//			if err != nil && e == nil {
//				e = err
//				cancel() // if any call failed, cancel unfinished calls
//			}
//			if err == nil && !replyDone {
//				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
//				replyDone = true
//			}
//			mu.Unlock()
//		}(rpcAddr)
//	}
//	wg.Wait()
//	cancel() // cancel unfinished calls
//	return e
//}
