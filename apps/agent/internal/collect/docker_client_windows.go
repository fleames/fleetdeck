//go:build windows

package collect

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/Microsoft/go-winio"
)

func dockerHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return winio.DialPipeContext(dctx, `\\.\pipe\docker_engine`)
		},
		ResponseHeaderTimeout: 3 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}
