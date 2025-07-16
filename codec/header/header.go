package header

type Header struct {
	ServiceMethod string //format "service.method"
	Seq           int64
	Error         string
}
