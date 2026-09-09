package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
)

type rateBucket struct {
	count   int
	resetAt time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]rateBucket
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{buckets: map[string]rateBucket{}, limit: limit, window: window}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok || now.After(b.resetAt) {
		rl.buckets[key] = rateBucket{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	rl.buckets[key] = b
	return true
}

func clientIP(r *http.Request) string {
	// Prefer Cloudflare's connecting IP when behind Tunnel / CF proxy.
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		return cf
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func roleAllowed(role string, roles ...string) bool {
	for _, r := range roles {
		if role == r {
			return true
		}
	}
	return false
}

func (s *Server) requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return s.requireUser(func(w http.ResponseWriter, r *http.Request) {
			u := r.Context().Value(ctxUser).(auth.User)
			if !roleAllowed(u.Role, roles...) {
				httpx.Error(w, http.StatusForbidden, "forbidden", "Your role cannot perform this action.")
				return
			}
			next(w, r)
		})
	}
}

func maxBody(bytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, bytes)
			next.ServeHTTP(w, r)
		})
	}
}
