package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFExempt(t *testing.T) {
	if !csrfExempt("/api/v1/auth/login") {
		t.Fatal("login should be exempt")
	}
	if !csrfExempt("/agent/v1/metrics") {
		t.Fatal("agent routes should be exempt")
	}
	if !csrfExempt("/install.sh") {
		t.Fatal("install.sh should be exempt")
	}
	if csrfExempt("/api/v1/servers") {
		t.Fatal("servers should not be exempt")
	}
}

func TestRequireCSRFBlocksMutationWithoutToken(t *testing.T) {
	s := &Server{}
	h := s.requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got %d", rr.Code)
	}
}

func TestRequireCSRFAllowsMatchingToken(t *testing.T) {
	s := &Server{}
	h := s.requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc123token"})
	req.Header.Set(csrfHeader, "abc123token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRequireCSRFAllowsGET(t *testing.T) {
	s := &Server{}
	h := s.requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
}
