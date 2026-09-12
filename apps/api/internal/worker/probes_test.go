package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeTCPValidation(t *testing.T) {
	ok, msg := probeTCP(context.Background(), "not-a-port", time.Second)
	if ok || msg == "" {
		t.Fatalf("expected invalid tcp target, got ok=%v msg=%q", ok, msg)
	}
	ok, msg = probeTCP(context.Background(), "https://example.com", time.Second)
	if ok || msg == "" {
		t.Fatalf("expected reject URL, got ok=%v msg=%q", ok, msg)
	}
}

func TestProbeHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ok, msg := probeHTTP(context.Background(), http.MethodGet, srv.URL, 204, 2*time.Second)
	if !ok {
		t.Fatalf("expected up, got %q", msg)
	}
	ok, msg = probeHTTP(context.Background(), http.MethodGet, srv.URL, 200, 2*time.Second)
	if ok {
		t.Fatalf("expected status mismatch down, got ok with %q", msg)
	}
	ok, msg = probeHTTP(context.Background(), http.MethodGet, "not-a-url", 200, time.Second)
	if ok || msg == "" {
		t.Fatalf("expected invalid URL, got ok=%v msg=%q", ok, msg)
	}
}

func TestTruncateErr(t *testing.T) {
	out := truncateErr(strings.Repeat("a", 600))
	if !strings.HasSuffix(out, "…") || len(out) > 520 {
		t.Fatalf("unexpected truncate: len=%d out=%q", len(out), out[:min(20, len(out))])
	}
}
