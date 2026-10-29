package rmm

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Hub tracks dial-out agent tunnels and per-session local RDP listeners for guacd/native clients.
type Hub struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*SessionTunnel
}

// SessionTunnel bridges one guacd/native TCP client to an agent WebSocket that forwards local RDP.
type SessionTunnel struct {
	ID            uuid.UUID
	HostID        uuid.UUID
	AgentConn     *websocket.Conn
	Listener      net.Listener
	ListenAddr    string // host:port reachable by guacd (e.g. api:49152)
	AdvertiseHost string

	muActive     sync.Mutex
	activeClient net.Conn
	writeMu      sync.Mutex

	closed    chan struct{}
	closeOnce sync.Once
}

func NewHub() *Hub {
	return &Hub{sessions: make(map[uuid.UUID]*SessionTunnel)}
}

func (h *Hub) Get(id uuid.UUID) *SessionTunnel {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[id]
}

func (h *Hub) Close(id uuid.UUID) {
	h.mu.Lock()
	st := h.sessions[id]
	delete(h.sessions, id)
	h.mu.Unlock()
	if st != nil {
		st.Close()
	}
}

func (st *SessionTunnel) Closed() <-chan struct{} { return st.closed }

func (st *SessionTunnel) Close() {
	st.closeOnce.Do(func() {
		close(st.closed)
		if st.Listener != nil {
			_ = st.Listener.Close()
		}
		if st.AgentConn != nil {
			_ = st.AgentConn.Close()
		}
		st.muActive.Lock()
		if st.activeClient != nil {
			_ = st.activeClient.Close()
			st.activeClient = nil
		}
		st.muActive.Unlock()
	})
}

// AttachAgent binds an agent WebSocket and opens a local TCP listener that proxies raw RDP.
// advertiseHost is the hostname guacd should dial (compose service name, e.g. "api").
// When advertiseHost is loopback, bind loopback only so the ephemeral RDP port is not LAN-exposed.
func (h *Hub) AttachAgent(sessionID, hostID uuid.UUID, conn *websocket.Conn, advertiseHost string) (*SessionTunnel, error) {
	bind := "0.0.0.0:0"
	if advertiseHost == "" || advertiseHost == "127.0.0.1" || advertiseHost == "localhost" {
		bind = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	tcpAddr := ln.Addr().(*net.TCPAddr)
	if advertiseHost == "" {
		advertiseHost = "127.0.0.1"
	}
	st := &SessionTunnel{
		ID:            sessionID,
		HostID:        hostID,
		AgentConn:     conn,
		Listener:      ln,
		ListenAddr:    fmt.Sprintf("%s:%d", advertiseHost, tcpAddr.Port),
		AdvertiseHost: advertiseHost,
		closed:        make(chan struct{}),
	}
	h.mu.Lock()
	if old := h.sessions[sessionID]; old != nil {
		h.mu.Unlock()
		_ = ln.Close()
		old.Close()
		return nil, fmt.Errorf("session already attached")
	}
	h.sessions[sessionID] = st
	h.mu.Unlock()

	go st.acceptLoop()
	go st.readAgentLoop()
	return st, nil
}

func (st *SessionTunnel) acceptLoop() {
	for {
		client, err := st.Listener.Accept()
		if err != nil {
			select {
			case <-st.closed:
				return
			default:
				log.Printf("rmm session %s accept: %v", st.ID, err)
				return
			}
		}
		go st.proxyClient(client)
	}
}

func (st *SessionTunnel) writeAgentJSON(v any) error {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()
	return st.AgentConn.WriteJSON(v)
}

func (st *SessionTunnel) writeAgentBinary(b []byte) error {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()
	return st.AgentConn.WriteMessage(websocket.BinaryMessage, b)
}

func (st *SessionTunnel) proxyClient(client net.Conn) {
	defer client.Close()
	_ = st.writeAgentJSON(map[string]any{"type": "open_rdp"})

	st.muActive.Lock()
	if st.activeClient != nil {
		_ = st.activeClient.Close()
	}
	st.activeClient = client
	st.muActive.Unlock()

	buf := make([]byte, 32*1024)
	for {
		n, err := client.Read(buf)
		if n > 0 {
			if werr := st.writeAgentBinary(buf[:n]); werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}

	st.muActive.Lock()
	if st.activeClient == client {
		st.activeClient = nil
	}
	st.muActive.Unlock()
	_ = st.writeAgentJSON(map[string]any{"type": "close_rdp"})
}

func (st *SessionTunnel) readAgentLoop() {
	for {
		mt, data, err := st.AgentConn.ReadMessage()
		if err != nil {
			st.Close()
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		st.muActive.Lock()
		c := st.activeClient
		st.muActive.Unlock()
		if c == nil {
			continue
		}
		if _, err := c.Write(data); err != nil {
			_ = c.Close()
		}
	}
}

// Pipe is unused helper kept for native TCP path symmetry.
func Pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(a, b); _ = a.Close() }()
	go func() { defer wg.Done(); _, _ = io.Copy(b, a); _ = b.Close() }()
	wg.Wait()
}
