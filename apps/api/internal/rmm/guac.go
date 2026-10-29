package rmm

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// EncodeGuacInstruction builds a Guacamole wire instruction.
func EncodeGuacInstruction(opcode string, args ...string) []byte {
	var b strings.Builder
	b.WriteString(strconv.Itoa(len(opcode)))
	b.WriteByte('.')
	b.WriteString(opcode)
	for _, a := range args {
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(len(a)))
		b.WriteByte('.')
		b.WriteString(a)
	}
	b.WriteByte(';')
	return []byte(b.String())
}

type guacInstruction struct {
	Opcode string
	Args   []string
}

func parseGuacInstruction(raw []byte) (guacInstruction, error) {
	data := string(raw)
	var elements []string
	i := 0
	for i < len(data) {
		dot := strings.IndexByte(data[i:], '.')
		if dot < 0 {
			return guacInstruction{}, fmt.Errorf("incomplete guac instruction")
		}
		dot += i
		length, err := strconv.Atoi(data[i:dot])
		if err != nil {
			return guacInstruction{}, fmt.Errorf("bad guac length: %w", err)
		}
		start := dot + 1
		end := start + length
		if end >= len(data) {
			return guacInstruction{}, fmt.Errorf("guac element overflows buffer")
		}
		elements = append(elements, data[start:end])
		term := data[end]
		i = end + 1
		if term == ';' {
			break
		}
		if term != ',' {
			return guacInstruction{}, fmt.Errorf("bad guac terminator %q", term)
		}
	}
	if len(elements) == 0 {
		return guacInstruction{}, fmt.Errorf("empty guac instruction")
	}
	return guacInstruction{Opcode: elements[0], Args: elements[1:]}, nil
}

// readGuacInstruction reads one full instruction from guacd.
func readGuacInstruction(conn net.Conn, deadline time.Duration) (guacInstruction, []byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(deadline))
	defer conn.SetReadDeadline(time.Time{})

	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		// Prefer parsing what we already have.
		if raw, ok := extractOneInstruction(buf.Bytes()); ok {
			inst, err := parseGuacInstruction(raw)
			return inst, raw, err
		}
		n, err := conn.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			return guacInstruction{}, nil, err
		}
		if buf.Len() > 1<<20 {
			return guacInstruction{}, nil, fmt.Errorf("guac instruction too large")
		}
	}
}

func extractOneInstruction(b []byte) ([]byte, bool) {
	// Walk length-prefixed elements until ';' terminator.
	i := 0
	for i < len(b) {
		dot := bytes.IndexByte(b[i:], '.')
		if dot < 0 {
			return nil, false
		}
		dot += i
		length, err := strconv.Atoi(string(b[i:dot]))
		if err != nil {
			return nil, false
		}
		end := dot + 1 + length
		if end >= len(b) {
			return nil, false
		}
		term := b[end]
		i = end + 1
		if term == ';' {
			return b[:i], true
		}
		if term != ',' {
			return nil, false
		}
	}
	return nil, false
}

// HandshakeGuacd performs the guacd handshake for an RDP connection using
// query parameters from guacamole-common-js (hostname, port, username, ...).
func HandshakeGuacd(conn net.Conn, q url.Values) (connectionID string, err error) {
	deadline := 20 * time.Second
	if _, err := conn.Write(EncodeGuacInstruction("select", "rdp")); err != nil {
		return "", err
	}
	argsInst, _, err := readGuacInstruction(conn, deadline)
	if err != nil {
		return "", fmt.Errorf("guacd args: %w", err)
	}
	if argsInst.Opcode != "args" {
		return "", fmt.Errorf("guacd expected args, got %q", argsInst.Opcode)
	}

	width := q.Get("width")
	if width == "" {
		width = "1280"
	}
	height := q.Get("height")
	if height == "" {
		height = "800"
	}
	dpi := q.Get("dpi")
	if dpi == "" {
		dpi = "96"
	}

	if _, err := conn.Write(EncodeGuacInstruction("size", width, height, dpi)); err != nil {
		return "", err
	}
	if _, err := conn.Write(EncodeGuacInstruction("audio", "audio/L16")); err != nil {
		return "", err
	}
	if _, err := conn.Write(EncodeGuacInstruction("video")); err != nil {
		return "", err
	}
	if _, err := conn.Write(EncodeGuacInstruction("image", "image/png", "image/jpeg")); err != nil {
		return "", err
	}
	if _, err := conn.Write(EncodeGuacInstruction("timezone", "UTC")); err != nil {
		return "", err
	}

	values := make([]string, 0, len(argsInst.Args))
	for _, name := range argsInst.Args {
		if strings.HasPrefix(name, "VERSION_") {
			values = append(values, name)
			continue
		}
		values = append(values, q.Get(name))
	}
	if _, err := conn.Write(EncodeGuacInstruction("connect", values...)); err != nil {
		return "", err
	}

	ready, _, err := readGuacInstruction(conn, deadline)
	if err != nil {
		return "", fmt.Errorf("guacd ready: %w", err)
	}
	if ready.Opcode != "ready" || len(ready.Args) < 1 {
		return "", fmt.Errorf("guacd expected ready, got %q %v", ready.Opcode, ready.Args)
	}
	return ready.Args[0], nil
}

// PipeWSToGuacd upgrades the guacamole-common-js expectation:
// server completes guacd handshake from query params, sends tunnel UUID,
// then pipes Guacamole protocol bytes both ways.
//
// guacamole-common-js WebSocketTunnel requires each WebSocket message to contain
// only complete Guacamole instructions (it does not buffer across messages).
// Internal empty-opcode messages (e.g. ping) are handled here and must not be
// forwarded to guacd — matching Apache Guacamole's WebSocket tunnel endpoint.
func PipeWSToGuacd(ws *websocket.Conn, guacd net.Conn, q url.Values) error {
	if _, err := HandshakeGuacd(guacd, q); err != nil {
		_ = ws.WriteMessage(websocket.TextMessage,
			EncodeGuacInstruction("error", err.Error(), "519"))
		return err
	}

	var wsWriteMu sync.Mutex
	writeWS := func(data []byte) error {
		wsWriteMu.Lock()
		defer wsWriteMu.Unlock()
		return ws.WriteMessage(websocket.TextMessage, data)
	}

	// guacamole-common-js WebSocketTunnel expects an empty-opcode UUID instruction first.
	tunnelID := uuid.NewString()
	if err := writeWS(EncodeGuacInstruction("", tunnelID)); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)

	go func() {
		defer wg.Done()
		defer guacd.Close()
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
				continue
			}
			// Tunnel-internal opcodes (empty string) stay on the WS tunnel.
			if inst, perr := parseGuacInstruction(data); perr == nil && inst.Opcode == "" {
				if len(inst.Args) >= 2 && inst.Args[0] == "ping" {
					if werr := writeWS(data); werr != nil {
						errCh <- werr
						return
					}
				}
				continue
			}
			if _, err := guacd.Write(data); err != nil {
				errCh <- err
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer ws.Close()
		var pending []byte
		buf := make([]byte, 32*1024)
		for {
			n, err := guacd.Read(buf)
			if n > 0 {
				pending = append(pending, buf[:n]...)
				var out []byte
				for {
					raw, ok := extractOneInstruction(pending)
					if !ok {
						break
					}
					if inst, perr := parseGuacInstruction(raw); perr == nil && inst.Opcode == "error" {
						msg := "guacd error"
						if len(inst.Args) > 0 && inst.Args[0] != "" {
							msg = inst.Args[0]
						}
						out = append(out, raw...)
						pending = pending[len(raw):]
						if len(out) > 0 {
							_ = writeWS(out)
						}
						errCh <- fmt.Errorf("%s", msg)
						return
					}
					out = append(out, raw...)
					pending = pending[len(raw):]
				}
				if len(out) > 0 {
					if werr := writeWS(out); werr != nil {
						errCh <- werr
						return
					}
				}
				if len(pending) > 16<<20 {
					errCh <- fmt.Errorf("guac instruction buffer overflow")
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					errCh <- err
				}
				return
			}
		}
	}()

	wg.Wait()
	select {
	case err := <-errCh:
		if err != nil && err != io.EOF {
			return err
		}
	default:
	}
	return nil
}
