package storeEngine

import (
	"discovery/dicovery"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Store interface {
	CreateService(service *Service) (reply []byte)
	AddEndpoint(endpoint *Endpoint) (reply []byte)
	UpdateEndpoints(endpoint *Endpoint) (reply []byte)
	Submit(command) (reply []byte)
	GC()
	WatchStream(target string) chan []byte
}
type command struct {
	cmdType     string
	serviceName string
	args        interface{}
	reply       interface{}
}

const (
	DeleteServiceCmdType  = "deleteService"
	AddServiceCmdType     = "addService"
	DeleteEndpointCmdType = "deleteEndpoint"
	AddEndpointCmdType    = "addEndpoint"
	UpdateEndpointCmdType = "updateEndpoint"
	UpdateServiceCmdType  = "updateService"
)

func formatCommand(cmdType string, serviceName string, args interface{}) command {
	return command{cmdType: cmdType, serviceName: serviceName, args: args}
}

var storeOnce sync.Once
var defaultStore *ServiceStore
var (
	ErrServiceExist     = errors.New("service already exists")
	ErrServiceNotExist  = errors.New("service not exists")
	ErrOldEndpoint      = errors.New("this endpoint is old endpoint")
	ErrEndpointNotExist = errors.New("endpoint not exists")
)

type ServiceStore struct {
	serviceMu   sync.RWMutex
	services    map[string]*Service
	expiredTime time.Duration
	outChanMap  map[string]chan []byte
}

func getStore() *ServiceStore {
	storeOnce.Do(func() {
		defaultStore = &ServiceStore{
			services:   make(map[string]*Service),
			outChanMap: make(map[string]chan []byte),
		}
	})
	return defaultStore
}
func (s *ServiceStore) WatchStream(target string) chan []byte {
	if s.outChanMap == nil {
		s.outChanMap = make(map[string]chan []byte)
	}
	if s.outChanMap[target] == nil {
		s.outChanMap[target] = make(chan []byte, 1024)
	}
	return s.outChanMap[target]
}
func (s *ServiceStore) Submit(cmd command) (reply []byte) {
	var (
		err      error
		event    = dicovery.Event{}
		endpoint = dicovery.Endpoint{}
	)
	data, err := json.Marshal(cmd.args)
	if err != nil {
		fmt.Println(cmd.cmdType, "submit error", err)
		reply = []byte(err.Error())
		return
	}

	// 发送回调信息给上层的channel
	if s.outChanMap[cmd.serviceName] == nil {
		s.outChanMap[cmd.serviceName] = make(chan []byte, 1024)
	}
	ch := s.outChanMap[cmd.serviceName]

	switch cmd.cmdType {
	case AddServiceCmdType:
		var service Service
		err = json.Unmarshal(data, &service)
		if err != nil {
			fmt.Println("Add service error", err)
			reply = []byte(err.Error())
			return
		}
		s.serviceMu.Lock()
		defer s.serviceMu.Unlock()
		s.services[cmd.serviceName] = &service

		reply, _ = json.Marshal(fmt.Sprintf("add service %s success", cmd.serviceName))
	case DeleteServiceCmdType:
		s.serviceMu.Lock()
		defer s.serviceMu.Unlock()
		delete(s.services, cmd.serviceName)

		reply, _ = json.Marshal("Delete service success")
	case UpdateServiceCmdType:
		s.serviceMu.Lock()
		defer s.serviceMu.Unlock()
		if _, exists := s.services[cmd.serviceName]; exists {
			s.services[cmd.serviceName].HeartBeat()
			reply, _ = json.Marshal("Update service success")
		} else {
			reply = []byte(ErrServiceNotExist.Error())
		}

	case AddEndpointCmdType:
		fmt.Println("add endpoint service name:", cmd.serviceName)
		var EP Endpoint
		err = json.Unmarshal(data, &EP)
		if err != nil {
			reply = []byte(err.Error())
			return
		}

		s.serviceMu.Lock()
		defer s.serviceMu.Unlock()
		if _, exists := s.services[cmd.serviceName]; exists {
			EP.owner = s.services[cmd.serviceName]
			s.services[cmd.serviceName].Endpoints[EP.URL] = &EP
			s.services[cmd.serviceName].Endpoints[EP.URL].HeartBeat()

			endpoint.ServiceName = cmd.serviceName
			endpoint.URL = EP.URL
			endpoint.Protocol = EP.Protocol
			event.Type = dicovery.Add
			event.Key = EP.URL
			event.Value, _ = json.Marshal(endpoint)
			messageBytes, _ := json.Marshal(event)
			ch <- messageBytes

			reply, _ = json.Marshal("Add endpoint success")
		} else {
			reply = []byte(ErrServiceNotExist.Error())
		}

	case DeleteEndpointCmdType:
		var EP Endpoint
		err = json.Unmarshal(data, &EP)
		if err != nil {
			reply = []byte(err.Error())
			return
		}
		s.serviceMu.Lock()
		defer s.serviceMu.Unlock()
		if _, exists := s.services[cmd.serviceName]; exists {
			delete(s.services[cmd.serviceName].Endpoints, EP.URL)

			event.Type = dicovery.Del
			event.Key = EP.URL
			messageBytes, _ := json.Marshal(event)
			ch <- messageBytes
		}

		reply, _ = json.Marshal("Delete endpoint success")
	case UpdateEndpointCmdType:
		var (
			eps []Endpoint
			EP  Endpoint
		)
		err = json.Unmarshal(data, &EP)
		if err != nil {
			reply = []byte(err.Error())
			return
		}
		s.serviceMu.Lock()
		defer s.serviceMu.Unlock()
		if _, exists := s.services[cmd.serviceName]; exists {
			for _, ep := range s.services[cmd.serviceName].Endpoints {
				eps = append(eps, *ep)
			}

			event.Type = dicovery.Update
			event.Key = cmd.serviceName
			event.Value, _ = json.Marshal(eps)

			messageBytes, _ := json.Marshal(event)
			ch <- messageBytes

			reply, _ = json.Marshal("Update endpoint success")
			return
		}
		reply, _ = json.Marshal(ErrEndpointNotExist.Error())
	default:
		err = errors.New("unknown command type")
		reply = []byte(err.Error())
	}
	return
}
func (s *ServiceStore) CreateService(service *Service) (reply []byte) {
	if _service, exists := s.services[service.Name]; exists {
		// 检查是否过期
		if _service.StartTime.Add(s.expiredTime).Before(time.Now()) {
			// 过期，更新
			//delete(s.services, service.ServiceName)
			return s.Submit(formatCommand(UpdateServiceCmdType, service.Name, nil))
		} else {
			// 未过期，退出
			reply = []byte(ErrServiceExist.Error())
			return
		}
	}
	//s.services[service.Name] = service
	return s.Submit(formatCommand(AddServiceCmdType, service.Name, service))
}
func (s *ServiceStore) AddEndpoint(endpoint *Endpoint) (reply []byte) {
	if s.services[endpoint.ServiceName] == nil {
		fmt.Println("service not exist", endpoint.ServiceName)
		reply = []byte(ErrServiceNotExist.Error())
		return
	}
	// 新节点，添加

	//s.services[endpoint.ServiceName].Endpoints[endpoint.URL] = endpoint
	//endpoint.HeartBeat()
	return s.Submit(formatCommand(AddEndpointCmdType, endpoint.ServiceName, endpoint))
}

// UpdateEndpoints 获取服务的endpoints
func (s *ServiceStore) UpdateEndpoints(endpoint *Endpoint) (reply []byte) {
	if s.services[endpoint.ServiceName] == nil {
		reply = []byte(endpoint.ServiceName + " " + ErrServiceNotExist.Error())
		return
	}
	return s.Submit(formatCommand(UpdateEndpointCmdType, endpoint.ServiceName, endpoint))
}
func (s *ServiceStore) GC() {

}
