package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientIPCloudflare(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if got := clientIP(req); got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIPRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:9999"
	if got := clientIP(req); got != "192.0.2.1" {
		t.Fatalf("got %q", got)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.allow("ip1") {
			t.Fatalf("expected allow on attempt %d", i+1)
		}
	}
	if rl.allow("ip1") {
		t.Fatal("expected deny after limit")
	}
	if !rl.allow("ip2") {
		t.Fatal("expected other key allowed")
	}
}

func TestSecureHeaders(t *testing.T) {
	s := &Server{cfg: testCfg()}
	h := s.secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing frame options")
	}
	if rr.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
	if rr.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS should be off when CookieSecure=false")
	}
}

func TestSecureHeadersHSTS(t *testing.T) {
	cfg := testCfg()
	cfg.CookieSecure = true
	s := &Server{cfg: cfg}
	h := s.secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil))
	if !strings.Contains(rr.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Fatal("expected HSTS when CookieSecure")
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	rl := newRateLimiter(50, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rl.allow("x")
		}()
	}
	wg.Wait()
}
