package dicovery

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect     SelectMode = iota // select randomly
	RoundRobinSelect                   // select using Robbin algorithm

)

type Discovery interface {
	Refresh() error // refresh from remote registry
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}
type MultiServersDiscovery struct {
	r       *rand.Rand   // generate random number
	mu      sync.RWMutex // protect following
	servers []string
	index   int // record the selected position for robin algorithm
}

// NewMultiServerDiscovery creates a MultiServersDiscovery instance
func NewMultiServerDiscovery(servers []string) *MultiServersDiscovery {
	d := &MultiServersDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.index = d.r.Intn(math.MaxInt32 - 1)
	return d
}

var _ Discovery = (*MultiServersDiscovery)(nil)

// Refresh doesn't make sense for MultiServersDiscovery, so ignore it
func (d *MultiServersDiscovery) Refresh() error {
	return nil
}

// Update the servers of discovery dynamically if needed
func (d *MultiServersDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}

// Get a server according to mode
func (d *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return d.servers[d.r.Intn(n)], nil
	case RoundRobinSelect:
		s := d.servers[d.index%n] // servers could be updated, so mode n to ensure safety
		d.index = (d.index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: not supported select mode")
	}
}

// returns all servers in discovery
func (d *MultiServersDiscovery) GetAll() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// return a copy of d.servers
	servers := make([]string, len(d.servers), len(d.servers))
	copy(servers, d.servers)
	return servers, nil
}

type GeeRegistryDiscovery struct {
	*MultiServersDiscovery
	registry   string
	timeout    time.Duration
	lastUpdate time.Time
}

const defaultUpdateTimeout = time.Second * 10

func NewGeeRegistryDiscovery(registerAddr string, timeout time.Duration) *GeeRegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	d := &GeeRegistryDiscovery{
		MultiServersDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:              registerAddr,
		timeout:               timeout,
	}
	return d
}
func (d *GeeRegistryDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	d.lastUpdate = time.Now()
	return nil
}

func (d *GeeRegistryDiscovery) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		//没有过期，不需要刷新
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", d.registry)
	resp, err := http.Get(d.registry)
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}
	servers := strings.Split(resp.Header.Get("X-Geerpc-Servers"), ",")
	d.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			d.servers = append(d.servers, strings.TrimSpace(server))
		}
	}
	d.lastUpdate = time.Now()
	return nil
}

func (d *GeeRegistryDiscovery) Get(mode SelectMode) (string, error) {
	if err := d.Refresh(); err != nil {
		return "", err
	}
	return d.MultiServersDiscovery.Get(mode)
}

func (d *GeeRegistryDiscovery) GetAll() ([]string, error) {
	if err := d.Refresh(); err != nil {
		return nil, err
	}
	return d.MultiServersDiscovery.GetAll()
}

type MyRPCDiscovery struct {
	ems          map[string]*EndpointManager
	emsMu        sync.RWMutex
	emsCancel    map[string]context.CancelFunc
	registryAddr string

	ctx        context.Context
	CancelFunc context.CancelFunc
}

var myRPCDiscovery MyRPCDiscovery
var disOnce sync.Once

func GetMyRPCDiscovery() *MyRPCDiscovery {
	disOnce.Do(func() {
		myRPCDiscovery = MyRPCDiscovery{
			ems:       make(map[string]*EndpointManager),
			emsCancel: make(map[string]context.CancelFunc),
			ctx:       context.Background(),
		}
	})
	return &myRPCDiscovery
}
func (d *MyRPCDiscovery) Discover(registryAddr string) {
	d.registryAddr = registryAddr
}
func (d *MyRPCDiscovery) Get(target string) *Endpoint {
	ctx, cancel := context.WithCancel(d.ctx)
	d.emsMu.Lock()
	d.emsCancel[target] = cancel
	d.emsMu.Unlock()

	d.emsMu.Lock()
	if d.ems[target] == nil {

		log.Println("Creating endpoint manager for", target)

		em := NewEndpointManager(d.registryAddr, target)
		d.ems[target] = em

		go func() {
			select {
			case <-ctx.Done():
				fmt.Println("Canceling endpoint manager")
				em.Cancel()
			}
		}()

		em.Listen()

	}
	d.emsMu.Unlock()

	var index int
	d.emsMu.RLock()
	defer d.emsMu.RUnlock()
	l := len(d.ems[target].ReadEndpoints())
	if l == 0 {
		log.Println(target, "len is zero")
		return nil
	}
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano())).Intn(l)

	for _, endpoint := range d.ems[target].ReadEndpoints() {
		if index == seededRand {
			return &endpoint
		}
		index++
	}
	return nil
}
