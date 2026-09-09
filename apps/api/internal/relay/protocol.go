package relay

import "net/http"

const (
	TypeHello   = "hello"
	TypeHelloOK = "hello_ok"
	TypeReq     = "req"
	TypeRes     = "res"
	TypePing    = "ping"
	TypePong    = "pong"
	TypeError   = "error"
)

// MaxMessageBytes caps proxied bodies (inventory can be large).
const MaxMessageBytes = 24 << 20

// Msg is a WebSocket frame between the public relay and the home edge.
type Msg struct {
	Type   string      `json:"type"`
	ID     string      `json:"id,omitempty"`
	Token  string      `json:"token,omitempty"`
	Method string      `json:"method,omitempty"`
	Path   string      `json:"path,omitempty"`
	Header http.Header `json:"header,omitempty"`
	Body   []byte      `json:"body,omitempty"`
	Status int         `json:"status,omitempty"`
	Error  string      `json:"error,omitempty"`
}
