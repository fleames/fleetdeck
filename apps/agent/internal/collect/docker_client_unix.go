//go:build !windows

package collect

import (
	"context"
	"net"
	"net/http"
	"time"
)

func dockerHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, "unix", "/var/run/docker.sock")
		},
		ResponseHeaderTimeout: 3 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}
