package relay

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Server is the public agent relay. Agents hit /agent/v1/*; the home edge
// connects outbound to /edge/connect and serves those requests.
type Server struct {
	Token string

	mu     sync.Mutex
	edge   *edgeSession
	pending map[string]chan Msg
}

type edgeSession struct {
	conn *websocket.Conn
	writeMu sync.Mutex
}

func NewServer(token string) *Server {
	return &Server{
		Token:   strings.TrimSpace(token),
		pending: map[string]chan Msg{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		online := s.edgeOnline()
		status := http.StatusOK
		if !online {
			status = http.StatusServiceUnavailable
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":true,"service":"fleetdeck-relay","edge_online":` + boolStr(online) + `}`))
	})
	mux.HandleFunc("/edge/connect", s.handleEdgeConnect)
	mux.HandleFunc("/", s.handleAgentProxy)
	return mux
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (s *Server) edgeOnline() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.edge != nil
}

func (s *Server) handleEdgeConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := bearerToken(r)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if s.Token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(MaxMessageBytes)

	sess := &edgeSession{conn: conn}
	s.mu.Lock()
	if s.edge != nil {
		_ = s.edge.conn.Close()
	}
	s.edge = sess
	s.mu.Unlock()
	log.Printf("relay: edge connected from %s", r.RemoteAddr)

	defer func() {
		s.mu.Lock()
		if s.edge == sess {
			s.edge = nil
		}
		s.mu.Unlock()
		_ = conn.Close()
		log.Printf("relay: edge disconnected")
	}()

	_ = sess.writeJSON(Msg{Type: TypeHelloOK})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Msg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case TypePing:
			_ = sess.writeJSON(Msg{Type: TypePong})
		case TypeRes, TypeError:
			s.mu.Lock()
			ch := s.pending[msg.ID]
			if ch != nil {
				delete(s.pending, msg.ID)
			}
			s.mu.Unlock()
			if ch != nil {
				select {
				case ch <- msg:
				default:
				}
			}
		}
	}
}

func (s *Server) handleAgentProxy(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/agent/") {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	edge := s.edge
	s.mu.Unlock()
	if edge == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"edge_offline","message":"FleetDeck home is offline. Start FleetDeck on your PC."}}`))
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, MaxMessageBytes))
	if err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	id := uuid.NewString()
	ch := make(chan Msg, 1)
	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	hdr := http.Header{}
	for k, vals := range r.Header {
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "keep-alive" || lk == "transfer-encoding" || lk == "upgrade" {
			continue
		}
		for _, v := range vals {
			hdr.Add(k, v)
		}
	}
	fullPath := path
	if r.URL.RawQuery != "" {
		fullPath += "?" + r.URL.RawQuery
	}
	if err := edge.writeJSON(Msg{
		Type:   TypeReq,
		ID:     id,
		Method: r.Method,
		Path:   fullPath,
		Header: hdr,
		Body:   body,
	}); err != nil {
		http.Error(w, `{"error":{"code":"edge_write_failed","message":"Could not reach FleetDeck home."}}`, http.StatusBadGateway)
		return
	}

	ctx := r.Context()
	timer := time.NewTimer(55 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		http.Error(w, "client canceled", http.StatusRequestTimeout)
	case <-timer.C:
		http.Error(w, `{"error":{"code":"edge_timeout","message":"FleetDeck home timed out."}}`, http.StatusGatewayTimeout)
	case msg := <-ch:
		if msg.Type == TypeError {
			http.Error(w, msg.Error, http.StatusBadGateway)
			return
		}
		for k, vals := range msg.Header {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		if msg.Status == 0 {
			msg.Status = http.StatusOK
		}
		w.WriteHeader(msg.Status)
		_, _ = w.Write(msg.Body)
	}
}

func (e *edgeSession) writeJSON(msg Msg) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	_ = e.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return e.conn.WriteJSON(msg)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

// Run starts the public relay HTTP listener until ctx is canceled.
func Run(ctx context.Context, addr, token string) error {
	srv := NewServer(token)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	log.Printf("FleetDeck relay listening on %s", addr)
	err := httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
