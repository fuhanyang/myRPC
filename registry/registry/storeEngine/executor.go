package storeEngine

import (
	"context"
	"sync"
	"time"
)

type CmdHandler func(Store, *Command) []byte
type Executor struct {
	ctx        context.Context
	cancelFunc context.CancelFunc

	Ch    chan *Command
	store Store

	cmdHandlers map[string]CmdHandler

	gcTicker *time.Ticker
}

var exeOnce sync.Once
var executor *Executor
var defaultHandlers = map[string]CmdHandler{
	AddEndpointCmdType:    AddEndpoint,
	AddServiceCmdType:     CreateService,
	UpdateEndpointCmdType: UpdateEndpoints,
}

func getExecutor() *Executor {
	exeOnce.Do(func() {
		executor = &Executor{
			ctx:         context.Background(),
			cmdHandlers: defaultHandlers,
			store:       getStore(),
			Ch:          make(chan *Command, 10),
			gcTicker:    time.NewTicker(time.Minute),
		}
	})
	go executor.run()
	return executor
}
func (e *Executor) Watch(ctx context.Context, target string, Ch chan []byte) {
	var data []byte
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ctx.Done():
			return
		case data = <-e.store.WatchStream(target):
			Ch <- data
		}
	}
}
func (e *Executor) run() {
	for {
		select {
		// ...
		// 每隔一段时间批量回收过期的 key
		case <-e.gcTicker.C:
			e.store.GC()
			// 接收来自 trigger 发送的 cmd，并执行具体的操作
		case cmd := <-e.Ch:
			cmdFunc, ok := e.cmdHandlers[cmd.CmdType]
			if !ok {
				panic("unknown command type: " + cmd.CmdType)
			}
			// 将执行结果通过 receiver chan 发送给 trigger
			cmd.replyCh <- cmdFunc(e.store, cmd)
		}
	}
}

func AddEndpoint(store Store, cmd *Command) []byte {
	return store.AddEndpoint(cmd.endpoint)
}
func CreateService(store Store, cmd *Command) []byte {
	var reply []byte
	service := Service{
		Name:      cmd.endpoint.ServiceName,
		Endpoints: make(map[string]*Endpoint),
		Ttl:       5 * time.Minute,
		timeMu:    sync.RWMutex{},
		StartTime: time.Now(),
	}
	reply = store.CreateService(&service)
	return reply
}
func UpdateEndpoints(store Store, cmd *Command) []byte {
	return store.UpdateEndpoints(cmd.endpoint)
}
