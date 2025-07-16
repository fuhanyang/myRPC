package codec

import (
	"codec/header"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	HeaderSize = 4
)

type JsonCodec struct {
	conn io.ReadWriteCloser
}

var _ Codec = (*JsonCodec)(nil)

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	return &JsonCodec{
		conn: conn,
	}
}
func parseHeader(data []byte) []byte {
	h := make([]byte, HeaderSize)
	bodyLen := uint32(len(data))
	fmt.Println("parse bodyLen:", bodyLen)
	binary.BigEndian.PutUint32(h, bodyLen)
	return h
}
func (c *JsonCodec) ReadHeader(h *header.Header) error {
	_h := make([]byte, HeaderSize)
	_, err := io.ReadFull(c.conn, _h)
	if err != nil {
		return err
	}

	bodyLen := binary.BigEndian.Uint32(_h)
	fmt.Println("bodyLen:", bodyLen)
	data := make([]byte, bodyLen)
	_, err = io.ReadFull(c.conn, data)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, h)
}

func (c *JsonCodec) ReadBody(b interface{}) error {
	_h := make([]byte, HeaderSize)
	_, err := io.ReadFull(c.conn, _h)
	if err != nil {
		return err
	}

	bodyLen := binary.BigEndian.Uint32(_h)

	data := make([]byte, bodyLen)
	_, err = io.ReadFull(c.conn, data)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, b)
}
func writeData(writer io.Writer, header []byte, message []byte) error {
	_, err := writer.Write(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(message)
	if err != nil {
		return err
	}
	return nil
}
func (c *JsonCodec) Write(h *header.Header, body interface{}) error {
	var err error
	defer func() {
		if err != nil {
			_ = c.Close()
		}
	}()
	hData, err := json.Marshal(h)
	if err != nil {
		return err
	}
	err = writeData(c.conn, parseHeader(hData), hData)
	if err != nil {
		return err
	}

	bData, err := json.Marshal(body)
	if err != nil {
		return err
	}
	err = writeData(c.conn, parseHeader(bData), bData)
	if err != nil {
		return err
	}
	return nil
}
func (c *JsonCodec) Close() error {
	return c.conn.Close()
}
