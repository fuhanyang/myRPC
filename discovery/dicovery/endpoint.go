package dicovery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type Endpoint struct {
	ServiceName string
	URL         string
	Protocol    string
}

// 创建watcher，监听节点变化，管理节点信息

type EndpointManager struct {
	Watcher           *Watcher
	Target            string
	Endpoints         map[string]Endpoint
	endpointsMu       sync.RWMutex
	UniqueId          string
	ctx               context.Context
	Cancel            context.CancelFunc
	firstUpdateSignal chan bool
}

func (em *EndpointManager) ReadEndpoints() map[string]Endpoint {
	em.endpointsMu.RLock()
	defer em.endpointsMu.RUnlock()
	return em.Endpoints
}
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
func NewEndpointManager(registryAddr string, target string) *EndpointManager {
	em := new(EndpointManager)
	em.UniqueId = generateRandomString(5)
	em.firstUpdateSignal = make(chan bool, 1)
	em.Endpoints = make(map[string]Endpoint)
	em.Target = target

	conn, err := net.Dial("tcp", registryAddr)
	if err != nil {
		fmt.Println("Error connecting to registry:", registryAddr)
		panic(err)
	}
	fmt.Println(conn.LocalAddr().String(), "Connected to registry:", registryAddr)
	writer := bufio.NewWriter(conn)
	//向服务端发送自己的唯一ID以记录
	uidMessage := fmt.Sprintf("UID %s\n", em.UniqueId+"_"+target)

	_, err = writer.WriteString(uidMessage)
	if err != nil {
		fmt.Println("Error sending UID to server:", err)
		return nil
	}
	err = writer.Flush()
	if err != nil {
		fmt.Println("Error sending UID to server:", err)
		return nil
	}

	ctx := context.Background()
	em.ctx, em.Cancel = context.WithCancel(ctx)
	em.Watcher = NewWatcher(em.ctx, conn)
	return em
}
func (em *EndpointManager) Listen() {
	go func() {
		var once sync.Once
		for {
			select {
			case <-em.Watcher.CloseChan:
				fmt.Println("Watcher closed")
				return
			case <-em.ctx.Done():
				return
			case event := <-em.Watcher.UpChan:
				switch event.Type {
				case Add:
					var endpoint Endpoint
					err := json.Unmarshal(event.Value, &endpoint)
					if endpoint.ServiceName != em.Target {
						fmt.Println("Endpoint for wrong service:", endpoint.ServiceName, "expected:", em.Target)
						break
					}
					if err != nil {
						fmt.Println("Error unmarshalling json:", err)
						break
					}
					em.endpointsMu.Lock()
					em.Endpoints[event.Key] = endpoint
					em.endpointsMu.Unlock()
					fmt.Println(em.Target, len(em.Endpoints), "Endpoint added:", endpoint)
					//fmt.Println("Endpoints:", em.Endpoints)
				case Del:
					em.endpointsMu.Lock()
					delete(em.Endpoints, event.Key)
					em.endpointsMu.Unlock()
				case Update:
					var endpoints []Endpoint
					err := json.Unmarshal(event.Value, &endpoints)
					if err != nil {
						fmt.Println("Error unmarshalling json:", err)
						break
					}
					em.endpointsMu.Lock()
					for _, ep := range endpoints {
						if ep.ServiceName != em.Target {
							log.Println("Endpoint for wrong service:", ep.ServiceName, "expected:", em.Target)
							continue
						}
						em.Endpoints[ep.URL] = ep
					}
					em.endpointsMu.Unlock()
					log.Println(em.Target, len(em.Endpoints), "Endpoints updated", em.Endpoints)
					once.Do(func() {
						em.firstUpdateSignal <- true
					})
				}
			}
		}
	}()
	<-em.firstUpdateSignal
}
