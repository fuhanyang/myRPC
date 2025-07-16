package codec

import (
	"codec/header"
	"io"
	"sync"
)

type Codec interface {
	io.Closer
	ReadHeader(header *header.Header) error
	ReadBody(interface{}) error
	Write(*header.Header, interface{}) error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

const (
	GobType  Type = "application/gob"
	JSONType Type = "application/json"
)

var (
	NewCodecFuncMap map[Type]NewCodecFunc
	mu              sync.RWMutex
)

func GetFactory(t Type) NewCodecFunc {
	mu.RLock()
	defer mu.RUnlock()
	return NewCodecFuncMap[t]
}
func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
	NewCodecFuncMap[JSONType] = NewJsonCodec
}
