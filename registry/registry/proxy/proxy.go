package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"registry/registry/storeEngine"
	"registry/registry/watcher"
	"sync"
	"time"
)

type Proxy interface {
	ConnectStore(storeEngine.StoreEngine)
	CreateProxy(watcher.Watcher, string)
	DeleteProxy(uid string)
	DeleteTarget(string)
	gc()
	Close()
}

type DefaultProxy struct {
	storeEngine   storeEngine.StoreEngine
	watchers      map[string]watcher.Watcher
	watcherMu     sync.RWMutex
	CancelFuncMap map[string]context.CancelFunc
	ctxMap        map[string]context.Context
	ctxMapMu      sync.RWMutex
	ctx           context.Context
	isAvailable   bool
	once          sync.Once
	broadcastFlag map[string]bool
	broadcastMu   sync.RWMutex
}

var (
	once         sync.Once
	defaultProxy DefaultProxy
)

func GetDefaultProxy() Proxy {
	once.Do(func() {
		defaultProxy = DefaultProxy{
			watchers:      make(map[string]watcher.Watcher),
			CancelFuncMap: make(map[string]context.CancelFunc),
			ctxMap:        make(map[string]context.Context),
			broadcastFlag: make(map[string]bool),
			ctx:           context.Background(),
			isAvailable:   true,
		}
		go defaultProxy.gc()
	})
	return &defaultProxy
}
func (p *DefaultProxy) isAlive() {
	if !p.isAvailable {
		panic(errors.New("proxy is closed"))
	}
}
func (p *DefaultProxy) gc() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(10 * time.Second):
			//定期进行一次垃圾回收，删除过期的watcher
			p.watcherMu.Lock()
			for uid, w := range p.watchers {
				if !w.IsAlive() {
					delete(p.watchers, uid)
					defaultChanGroup.Del(w.GetTarget(), w.GetUid())
				}
			}
			p.watcherMu.Unlock()
		}
	}
}
func (p *DefaultProxy) Close() {
	p.once.Do(func() {
		p.ctx.Done()
		for _, cancel := range p.CancelFuncMap {
			cancel()
		}
		p.isAvailable = false
	})
}
func (p *DefaultProxy) ConnectStore(storeEngine storeEngine.StoreEngine) {
	//检验proxy是否可用
	p.isAlive()

	p.storeEngine = storeEngine

}

func (p *DefaultProxy) CreateProxy(watcher watcher.Watcher, target string) {
	//检验proxy是否可用
	p.isAlive()

	if watcher == nil {
		fmt.Println("watcher is nil")
		return
	}
	//通过ctxMap[target]来控制target范围下所有watcher的生命周期
	p.ctxMapMu.Lock()
	if p.ctxMap[target] == nil {
		p.ctxMap[target] = context.Background()
	}
	ctx, cancel := context.WithCancel(p.ctxMap[target])
	p.CancelFuncMap[target] = cancel
	p.ctxMapMu.Unlock()

	// 启动watch
	watcher.Watch(ctx, target)
	p.watcherMu.Lock()
	p.watchers[watcher.GetUid()] = watcher
	p.watcherMu.Unlock()

	// 将watcher的连接信息传递给storeEngine
	defaultChanGroup.Add(watcher.GetTarget(), watcher.GetUid(), watcher.Connect())

	// 启动一个goroutine，将target的消息广播到其对应的所有watcher,如果已经启动，则不再启动
	if ch := p.storeEngine.Connect(ctx, target); ch != nil {
		go p.broadcast(ctx, ch, defaultChanGroup.GetWatcherGroup(target), target)
	}

	return
}

// DeleteProxy 关闭指定uid的watcher，并从map中删除该watcher
func (p *DefaultProxy) DeleteProxy(uid string) {
	//检验proxy是否可用
	p.isAlive()

	// 关闭指定uid的watcher
	p.watcherMu.RLock()
	w := p.watchers[uid]
	p.watcherMu.RUnlock()
	if w != nil {
		w.Close()
		p.watcherMu.Lock()
		delete(p.watchers, uid)
		p.watcherMu.Unlock()
		defaultChanGroup.Del(w.GetTarget(), w.GetUid())
	}
}
func (p *DefaultProxy) DeleteTarget(target string) {
	//检验proxy是否可用
	p.isAlive()

	// 关闭指定target的所有watcher
	p.CancelFuncMap[target]()
}

// chanGroup 用于管理target和对应watcher group之间的关系
type chanGroup struct {
	group map[string]*watcherGroup
	mu    sync.RWMutex
}

// watcherGroup 用于管理一个watcher group中每个watcher的消息通道
type watcherGroup struct {
	mu    sync.RWMutex
	group map[string]chan []byte
}

var defaultChanGroup = chanGroup{group: make(map[string]*watcherGroup)}

func (cg *chanGroup) GetWatcherGroup(target string) *watcherGroup {
	cg.mu.RLock()
	defer cg.mu.RUnlock()
	return cg.group[target]
}

func (cg *chanGroup) Add(target string, uid string, ch chan []byte) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	if cg.group[target] == nil {
		cg.group[target] = &watcherGroup{group: make(map[string]chan []byte)}
	}
	cg.group[target].mu.Lock()
	defer cg.group[target].mu.Unlock()
	cg.group[target].group[uid] = ch
}
func (cg *chanGroup) Del(target string, uid string) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()
	if cg.group[target] == nil {
		return
	}
	cg.group[target].mu.Lock()
	defer cg.group[target].mu.Unlock()
	delete(cg.group[target].group, uid)
}
func (cg *chanGroup) Get(target string, uid string) chan []byte {
	cg.mu.RLock()
	defer cg.mu.RUnlock()
	if cg.group[target] == nil {
		return nil
	}
	cg.group[target].mu.RLock()
	defer cg.group[target].mu.RUnlock()
	return cg.group[target].group[uid]
}
func (p *DefaultProxy) broadcast(ctx context.Context, outChan chan []byte, group *watcherGroup, target string) {
	//如果已经启动，则不再启动
	p.broadcastMu.RLock()
	if p.broadcastFlag[target] {
		p.broadcastMu.RUnlock()
		return
	}
	p.broadcastMu.RUnlock()

	p.broadcastMu.Lock()
	p.broadcastFlag[target] = true
	p.broadcastMu.Unlock()

	defer func() {
		p.broadcastMu.Lock()
		p.broadcastFlag[target] = false
		p.broadcastMu.Unlock()
	}()

	for {

		select {
		case msg := <-outChan:

			group.mu.RLock()
			for uid, ch := range group.group {
				go func(uid string, ch chan []byte) {
					if !p.watchers[uid].IsAlive() {
						return
					}

					defer func() {
						if r := recover(); r != nil {
							log.Println("broadcast to a invalid watcher", uid)
						}
					}()
					ch <- msg
				}(uid, ch)
			}
			group.mu.RUnlock()
		case <-ctx.Done():
			return
		}

	}
}
