package storeEngine

type Command struct {
	CmdType  string
	endpoint *Endpoint
	replyCh  chan []byte
}

func NewCommand() *Command {
	return &Command{
		replyCh: make(chan []byte),
	}
}
func (c *Command) Receiver() <-chan []byte {
	if c.replyCh == nil {
		c.replyCh = make(chan []byte)
	}
	return c.replyCh
}
