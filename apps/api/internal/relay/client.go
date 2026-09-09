package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client dials the public relay and serves proxied agent requests against localURL.
type Client struct {
	RelayURL  string // e.g. wss://agents.tarkovbot.com/edge/connect
	Token     string
	LocalURL  string // e.g. http://127.0.0.1:8080
	HTTPClient *http.Client
}

// Run reconnects forever until ctx is canceled.
func (c *Client) Run(ctx context.Context) {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 55 * time.Second}
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Printf("relay edge: disconnected (%v); retry in %s", err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) session(ctx context.Context) error {
	u, err := url.Parse(c.RelayURL)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+c.Token)
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u.String(), hdr)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(MaxMessageBytes)
	log.Printf("relay edge: connected to %s", c.RelayURL)

	var writeMu sync.Mutex
	write := func(msg Msg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		return conn.WriteJSON(msg)
	}

	// keepalive pings
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := write(Msg{Type: TypePing}); err != nil {
					return
				}
			}
		}
	}()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg Msg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case TypeHelloOK, TypePong:
			continue
		case TypeReq:
			go c.handleReq(ctx, msg, write)
		}
	}
}

func (c *Client) handleReq(ctx context.Context, msg Msg, write func(Msg) error) {
	res := Msg{Type: TypeRes, ID: msg.ID}
	local := strings.TrimRight(c.LocalURL, "/") + msg.Path
	req, err := http.NewRequestWithContext(ctx, msg.Method, local, bytes.NewReader(msg.Body))
	if err != nil {
		res.Type = TypeError
		res.Error = err.Error()
		_ = write(res)
		return
	}
	for k, vals := range msg.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	// Ensure loopback host is local; drop forwarded host from agent.
	req.Host = ""
	req.RequestURI = ""

	httpRes, err := c.HTTPClient.Do(req)
	if err != nil {
		res.Type = TypeError
		res.Error = fmt.Sprintf("local api: %v", err)
		_ = write(res)
		return
	}
	defer httpRes.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpRes.Body, MaxMessageBytes))
	if err != nil {
		res.Type = TypeError
		res.Error = err.Error()
		_ = write(res)
		return
	}
	res.Status = httpRes.StatusCode
	res.Header = http.Header{}
	for k, vals := range httpRes.Header {
		lk := strings.ToLower(k)
		if lk == "transfer-encoding" || lk == "connection" {
			continue
		}
		for _, v := range vals {
			res.Header.Add(k, v)
		}
	}
	res.Body = body
	_ = write(res)
}
