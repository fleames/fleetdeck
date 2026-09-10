//go:build !windows

package collect

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	dockerClientOnce sync.Once
	dockerClient     *http.Client
)

// dockerHTTPClient returns a process-wide shared client. Creating a new
// http.Transport per request leaks idle connections and goroutines (observed
// as multi-GB RES growth under the metrics/inventory poll loops).
func dockerHTTPClient() *http.Client {
	dockerClientOnce.Do(func() {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := &net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "unix", "/var/run/docker.sock")
			},
			MaxIdleConns:          4,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
			DisableCompression:    true,
		}
		dockerClient = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	})
	return dockerClient
}
