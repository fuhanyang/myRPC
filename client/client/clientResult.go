package client

import (
	"fmt"
	"net"
	"server/server"
	"time"
)

type clientResult struct {
	client *Client
	err    error
}

type newClientFunc func(conn net.Conn, opt *server.Option) (client *Client, err error)

func dialTimeout(f newClientFunc, network, address string, opts ...*server.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult)
	go func() {
		client, err := f(conn, opt)
		ch <- clientResult{client: client, err: err}
	}()
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

// Dial connects to an RPC server at the specified network address
func Dial(network, address string, opts ...*server.Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opts...)
}
