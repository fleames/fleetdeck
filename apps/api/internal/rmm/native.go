package rmm

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// NativeGateway accepts TCP clients that send "AUTH <token>\n" then proxies raw RDP
// to the matching session tunnel listener (same path guacd uses).
type NativeGateway struct {
	Addr string
	Lookup func(ctx context.Context, token string) (sessionID uuid.UUID, listenAddr string, ok bool)
	OnConnect func(sessionID uuid.UUID)
	OnClose   func(sessionID uuid.UUID)

	mu   sync.Mutex
	ln   net.Listener
}

func (g *NativeGateway) Start(ctx context.Context) error {
	if strings.TrimSpace(g.Addr) == "" {
		return nil
	}
	ln, err := net.Listen("tcp", g.Addr)
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.ln = ln
	g.mu.Unlock()
	log.Printf("rmm native gateway listening on %s", g.Addr)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go g.acceptLoop(ctx, ln)
	return nil
}

func (g *NativeGateway) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("rmm native accept: %v", err)
				return
			}
		}
		go g.handle(ctx, c)
	}
}

func (g *NativeGateway) handle(ctx context.Context, client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(client)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "AUTH ") {
		_, _ = io.WriteString(client, "ERR auth required\n")
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(line, "AUTH "))
	if token == "" || g.Lookup == nil {
		_, _ = io.WriteString(client, "ERR invalid\n")
		return
	}
	sessionID, listenAddr, ok := g.Lookup(ctx, token)
	if !ok || listenAddr == "" {
		_, _ = io.WriteString(client, "ERR unknown\n")
		return
	}
	_, _ = io.WriteString(client, "OK\n")
	_ = client.SetDeadline(time.Time{})

	upstream, err := net.DialTimeout("tcp", listenAddr, 10*time.Second)
	if err != nil {
		_, _ = io.WriteString(client, "ERR dial\n")
		return
	}
	defer upstream.Close()
	if g.OnConnect != nil {
		g.OnConnect(sessionID)
	}
	if g.OnClose != nil {
		defer g.OnClose(sessionID)
	}

	// Leftover buffered bytes (shouldn't exist after AUTH line) go upstream.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, br)
		_ = upstream.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
	}()
	wg.Wait()
}
