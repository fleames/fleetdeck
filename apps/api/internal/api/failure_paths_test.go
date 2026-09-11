package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestLoginInvalidBody(t *testing.T) {
	s := &Server{cfg: testCfg()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{`))
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rr.Code)
	}
	assertErrorCode(t, rr, "invalid_body")
}

func TestExportUnknownKind(t *testing.T) {
	// route-level kind validation without DB: call handler with chi context empty → default case
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/nope", nil)
	rr := httptest.NewRecorder()
	s.handleExport(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRestoreRequiresConfirm(t *testing.T) {
	s := &Server{}
	body := bytes.NewBufferString(`{"version":1,"confirm":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore", body)
	req = req.WithContext(withUser(req.Context(), testAdmin()))
	rr := httptest.NewRecorder()
	s.handleRestore(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d body=%s", rr.Code, rr.Body.String())
	}
	assertErrorCode(t, rr, "confirmation_required")
}

func TestContainerActionRequiresConfirm(t *testing.T) {
	s := &Server{}
	body := bytes.NewBufferString(`{"confirm":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/{id}/actions/restart", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "00000000-0000-0000-0000-000000000001")
	rctx.URLParams.Add("action", "restart")
	req = req.WithContext(context.WithValue(withUser(req.Context(), testAdmin()), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	s.handleContainerAction(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d body=%s", rr.Code, rr.Body.String())
	}
	assertErrorCode(t, rr, "confirmation_required")
}

func TestClearStaleContainersRequiresConfirm(t *testing.T) {
	s := &Server{}
	body := bytes.NewBufferString(`{"confirm":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/clear-stale", body)
	req = req.WithContext(withUser(req.Context(), testAdmin()))
	rr := httptest.NewRecorder()
	s.handleClearStaleContainers(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d body=%s", rr.Code, rr.Body.String())
	}
	assertErrorCode(t, rr, "confirmation_required")
}

func TestCompareRequiresTwoServers(t *testing.T) {
	s := &Server{}
	body := bytes.NewBufferString(`{"server_ids":["11111111-1111-1111-1111-111111111111"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/compare", body)
	req = req.WithContext(withUser(req.Context(), testAdmin()))
	rr := httptest.NewRecorder()
	s.handleCompareServers(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rr.Code)
	}
	assertErrorCode(t, rr, "validation")
}

func TestWriteExportCSVHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	writeExport(rr, "csv", "servers", []string{"id", "name"}, [][]string{{"1", "a"}})
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("content-type=%s", ct)
	}
	if !strings.Contains(rr.Body.String(), "id,name") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func assertErrorCode(t *testing.T, rr *httptest.ResponseRecorder, code string) {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v body=%s", err, rr.Body.String())
	}
	if env.Error.Code != code {
		t.Fatalf("code=%s want %s body=%s", env.Error.Code, code, rr.Body.String())
	}
}
