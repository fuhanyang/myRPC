package registry

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"registry/registry/proxy"
	"registry/registry/storeEngine"
	"registry/registry/watcher"
	"sort"
	"strings"
	"sync"
	"time"
)

type MyRegistry struct {
	store    storeEngine.StoreEngine
	proxy    proxy.Proxy
	listener net.Listener
}

var defaultRegistry MyRegistry
var regiOnce sync.Once

func DefaultRegistry() *MyRegistry {
	regiOnce.Do(func() {
		defaultRegistry = MyRegistry{
			store: storeEngine.GetTrigger(),
			proxy: proxy.GetDefaultProxy(),
		}
		defaultRegistry.proxy.ConnectStore(defaultRegistry.store)
	})
	return &defaultRegistry
}

func (r *MyRegistry) Listen(network, address string) error {
	var err error
	r.listener, err = net.Listen(network, address)
	return err
}

func (r *MyRegistry) Start(ctx context.Context) {
	fmt.Println("registry start")
	if r.listener == nil {
		log.Println("listener is nil")
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				err := r.listener.Close()
				if err != nil {
					log.Println(err)
				}
				return
			default:
				conn, err := r.listener.Accept()
				if err != nil {
					log.Println(err)
					continue
				}
				go r.ServeConn(conn)
			}

		}
	}()
}
func (r *MyRegistry) ServeConn(conn net.Conn) {
	var err error
	// 读取客户端名称（或唯一标识符）
	reader := bufio.NewReader(conn)
	clientName, _ := reader.ReadString('\n')
	clientName = strings.TrimSpace(clientName)

	split := strings.Split(clientName, "_")
	if len(split) != 2 {
		panic("client name should be in format 'id_target'")
	}
	fmt.Println("client name:", clientName)

	// 启动一个 watcher 来监听服务端的变化
	w := watcher.NewWatcher(conn, split[0])
	r.proxy.CreateProxy(w, split[1])
	//默认更新一次数据
	reply, err := r.Update(context.Background(), split[1])
	if err != nil {
		log.Println(err)
	}
	log.Println("update reply:", string(reply))

	buffer := make([]byte, 1024)
	_, err = reader.Read(buffer)
	if err == io.EOF {
		fmt.Println("client disconnected", conn.RemoteAddr().String())
		w.Close()
	}
}
func (r *MyRegistry) Update(ctx context.Context, serviceName string) ([]byte, error) {
	reply, err := r.store.Do(ctx,
		storeEngine.UpdateEndpointCmdType,
		&storeEngine.Endpoint{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}
	return reply.ToBytes(), nil
}
func (r *MyRegistry) Register(ctx context.Context, serviceName string, url string, protocol string) ([]byte, error) {
	reply, err := r.store.Do(context.Background(),
		storeEngine.AddServiceCmdType,
		&storeEngine.Endpoint{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	reply, err = r.store.Do(ctx,
		storeEngine.AddEndpointCmdType,
		&storeEngine.Endpoint{
			ServiceName: serviceName,
			URL:         url,
			Protocol:    protocol,
		},
	)
	if err != nil {
		return nil, err
	}
	return reply.ToBytes(), nil
}

//

//

//

// GeeRegistry is a simple register center, provide following functions.
// add a server and receive heartbeat to keep it alive.
// returns all alive servers and delete dead servers sync simultaneously.
type GeeRegistry struct {
	timeout time.Duration
	mu      sync.Mutex // protect following
	servers map[string]*ServerItem
}

type ServerItem struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/_geerpc_/registry"
	defaultTimeout = time.Minute * 5
)

// New create a registry instance with timeout setting
func New(timeout time.Duration) *GeeRegistry {
	return &GeeRegistry{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultGeeRegister = New(defaultTimeout)

func (r *GeeRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil {
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else {
		s.start = time.Now() // if exists, update start time to keep alive
	}
}

func (r *GeeRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

// Runs at /_geerpc_/registry
func (r *GeeRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// keep it simple, server is in req.Header
		w.Header().Set("X-Geerpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		// keep it simple, server is in req.Header
		addr := req.Header.Get("X-Geerpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HandleHTTP registers an HTTP handler for GeeRegistry messages on registryPath
func (r *GeeRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultGeeRegister.HandleHTTP(defaultPath)
}

// Heartbeat send a heartbeat message every once in a while
// it's a helper function for a server to register or send heartbeat
func Heartbeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		// make sure there is enough time to send heart beat
		// before it's removed from registry
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHeartbeat(registry, addr)
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			err = sendHeartbeat(registry, addr)
		}
	}()
}

func sendHeartbeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-Geerpc-Server", addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		return
	}
	defer listener.Close()
	fmt.Println("Listening on port 8080...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting:", err.Error())
			continue
		}
		// 你可以在这里添加逻辑来从连接中获取客户端名称或其他唯一标识符
		// 在这个例子中，我们假设客户端在连接后立即发送其名称
		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()

	// 读取客户端名称（或唯一标识符）
	reader := bufio.NewReader(conn)
	clientName, _ := reader.ReadString('\n')
	clientName = strings.TrimSpace(clientName)

	split := strings.Split(clientName, "_")
	if len(split) != 2 {
		panic("client name should be in format 'id_target'")
	}
	// 启动一个 watcher 来监听服务端的变化
	proxy.GetDefaultProxy().CreateProxy(watcher.NewWatcher(conn, split[0]), split[1])
	// 为了示例简单，我们创建一个 goroutine 来监听来自服务器的消息（实际上应该是另一个协议或机制）
	go func() {
		for {
			// 假设我们有一个机制来获取要发送给此客户端的消息
			// 在这个例子中，我们简单地使用固定的消息
			message := fmt.Sprintf("Hello, %s!", clientName)
			conn.Write([]byte(message + "\n"))
			// 注意：在实际应用中，你应该有一个更好的消息传递机制，并且不应该在这个 goroutine 中无限循环
			// 你可能需要一个退出条件或信号通道来优雅地关闭这个 goroutine
			// 这里只是为了演示如何发送消息给客户端
			break // 移除这个 break 来模拟持续发送消息
		}
	}()

}
