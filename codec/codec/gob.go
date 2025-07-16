package codec

import (
	"bufio"
	"codec/header"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

func (c *GobCodec) ReadHeader(h *header.Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(b interface{}) error {
	return c.dec.Decode(b)
}

func (c *GobCodec) Write(h *header.Header, body interface{}) error {
	var err error
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err := c.enc.Encode(h); err != nil {
		log.Println("server codec: gob error encoding header", err)
		return err
	}
	if err := c.enc.Encode(body); err != nil {
		log.Println("server codec: gob error encoding body", err)
		return err
	}
	return nil
}
func (c *GobCodec) Close() error {
	return c.conn.Close()
}
