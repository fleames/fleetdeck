package collect

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

func FetchContainerLogs(ctx context.Context, containerID string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}
	path := "/containers/" + containerID + "/logs?stdout=true&stderr=true&timestamps=true&tail=" + strconv.Itoa(tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return "", err
	}
	res, err := dockerHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2_000_000))
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("docker logs HTTP %d: %s", res.StatusCode, string(body))
	}
	// Docker multiplexes stream headers when TTY is false; strip 8-byte frames when present.
	return string(demuxDockerLogs(body)), nil
}

func demuxDockerLogs(b []byte) []byte {
	if len(b) < 8 {
		return b
	}
	// Heuristic: if first byte is 0-2 and size prefix looks sane, demux.
	out := make([]byte, 0, len(b))
	i := 0
	for i+8 <= len(b) {
		stream := b[i]
		if stream > 2 {
			return b
		}
		size := int(b[i+4])<<24 | int(b[i+5])<<16 | int(b[i+6])<<8 | int(b[i+7])
		i += 8
		if size < 0 || i+size > len(b) {
			return b
		}
		out = append(out, b[i:i+size]...)
		i += size
	}
	if len(out) == 0 {
		return b
	}
	return out
}
