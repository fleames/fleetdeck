package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/realtime"
)

func TestRoleAllowedMatrix(t *testing.T) {
	cases := []struct {
		role    string
		allowed []string
		want    bool
	}{
		{"admin", []string{"admin", "operator"}, true},
		{"operator", []string{"admin", "operator"}, true},
		{"viewer", []string{"admin", "operator"}, false},
		{"viewer", []string{"admin"}, false},
		{"admin", []string{"admin"}, true},
	}
	for _, c := range cases {
		if got := roleAllowed(c.role, c.allowed...); got != c.want {
			t.Fatalf("roleAllowed(%q, %v)=%v want %v", c.role, c.allowed, got, c.want)
		}
	}
}

func TestRealtimeOriginOK(t *testing.T) {
	origins := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if !realtimeOriginOK("http://localhost:3000", origins, true) {
		t.Fatal("allowed origin should pass")
	}
	if realtimeOriginOK("http://evil.example", origins, true) {
		t.Fatal("foreign origin should fail")
	}
	if realtimeOriginOK("", origins, true) {
		t.Fatal("empty origin must fail when CookieSecure")
	}
	if !realtimeOriginOK("", origins, false) {
		t.Fatal("empty origin allowed for local CookieSecure=false")
	}
}

func TestHandleRealtimeUnauthenticated(t *testing.T) {
	s := &Server{
		cfg:  testCfg(),
		hub:  realtime.NewHub(),
		auth: auth.NewService(nil, false),
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/realtime", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	s.handleRealtime(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleRealtimeForbiddenOriginWhenSecure(t *testing.T) {
	cfg := testCfg()
	cfg.CookieSecure = true
	s := &Server{
		cfg:  cfg,
		hub:  realtime.NewHub(),
		auth: auth.NewService(nil, true),
	}
	// Still unauthenticated first — origin check is after auth.
	// Validate origin helper separately; here ensure empty origin rejected only after auth.
	// With no cookie we get 401 before origin.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/realtime", nil)
	s.handleRealtime(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before origin check, got %d", rr.Code)
	}
}

func TestMetricsHistorySource(t *testing.T) {
	src, max := metricsHistorySource("1h", 7, 30, 365)
	if src != "raw" || max != 7*24*time.Hour {
		t.Fatalf("1h => %s %v", src, max)
	}
	src, max = metricsHistorySource("24h", 7, 30, 365)
	if src != "5m" || max != 30*24*time.Hour {
		t.Fatalf("24h => %s %v", src, max)
	}
	src, max = metricsHistorySource("30d", 7, 30, 365)
	if src != "1h" || max != 365*24*time.Hour {
		t.Fatalf("30d => %s %v", src, max)
	}
}
