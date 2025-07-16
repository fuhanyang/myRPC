package storeEngine

import (
	"context"
	"discovery/dicovery"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var commands = map[string]bool{
	AddEndpointCmdType:    true,
	AddServiceCmdType:     true,
	UpdateEndpointCmdType: true,
}
var (
	ErrInvalidCommand = errors.New("invalid command")
)

type Trigger struct {
	once       sync.Once
	executor   *Executor
	OutChanMap map[string]chan []byte
}
type TriggerReply struct {
	content []byte
}

var trigger *Trigger
var triOnce sync.Once

func GetTrigger() StoreEngine {
	triOnce.Do(func() {
		trigger = &Trigger{
			executor:   getExecutor(),
			OutChanMap: make(map[string]chan []byte),
		}
	})
	return trigger
}

func (r *TriggerReply) ToBytes() []byte {
	return r.content
}

func (t *Trigger) Close() {
	t.once.Do(func() {
	})
}

func (t *Trigger) Do(ctx context.Context, cmd string, endpoint *Endpoint) (Reply, error) {
	var (
		_cmd  = NewCommand()
		reply = &TriggerReply{}
	)
	if !CheckCommandType(cmd) {
		fmt.Println("invalid command:", cmd)
		return reply, ErrInvalidCommand
	}
	_cmd.endpoint = endpoint
	_cmd.CmdType = cmd
	t.executor.Ch <- _cmd

	timer := time.NewTimer(time.Second * 5)
	defer timer.Stop()
	select {
	case <-timer.C:
		return reply, errors.New("timeout")
	case <-ctx.Done():
		return reply, ctx.Err()
	case reply.content = <-_cmd.Receiver():
		return reply, nil
	}
}

func CheckCommandType(cmd string) bool {
	return commands[cmd]
}
func (t *Trigger) Connect(ctx context.Context, target string) chan []byte {
	if t.OutChanMap == nil {
		t.OutChanMap = make(map[string]chan []byte)
	}
	if t.OutChanMap[target] != nil {
		return t.OutChanMap[target]
	}
	t.OutChanMap[target] = make(chan []byte, 1024)
	go t.executor.Watch(ctx, target, t.OutChanMap[target])
	return t.OutChanMap[target]
}
func (t *Trigger) Disconnect(target string) {

}

type TestStore struct {
	outChan chan []byte
}

func (t *TestStore) Do(ctx context.Context, cmd string, endpoint *Endpoint) (Reply, error) {

	return nil, nil
}
func (t *TestStore) Close() {

}
func (t *TestStore) Connect(ctx context.Context, target string) chan []byte {
	if t.outChan != nil {
		return t.outChan
	}
	t.outChan = make(chan []byte, 1024)
	go func() {
		var i int
		for {
			i++
			select {
			case <-ctx.Done():
				return
			default:
				var (
					event    = dicovery.Event{}
					endpoint = dicovery.Endpoint{ServiceName: "testService", URL: fmt.Sprintf("http://127.0.0.1:%d", i)}
					err      error
				)
				if i == 3 {
					event.Type = dicovery.Del
					event.Key = fmt.Sprintf("http://127.0.0.1:%d", i-1)
				} else if i%2 == 0 {
					event.Type = dicovery.Update
					event.Key = fmt.Sprintf("http://127.0.0.1:%d", i-1)
				} else {
					event.Key = endpoint.URL
					event.Type = dicovery.Add
				}
				event.Value, err = json.Marshal(endpoint)
				if err != nil {
					panic(err)
				}
				messageBytes, err := json.Marshal(event)
				if err != nil {
					panic(err)
				}
				t.outChan <- messageBytes
			}
			time.Sleep(time.Second * 3)
		}
	}()
	return t.outChan
}
