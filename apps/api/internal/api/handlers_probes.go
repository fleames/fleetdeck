package api

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxUptimeProbes = 20

type uptimeProbeRow struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	Enabled          bool       `json:"enabled"`
	Kind             string     `json:"kind"`
	Target           string     `json:"target"`
	Method           string     `json:"method"`
	ExpectedStatus   int        `json:"expected_status"`
	IntervalSeconds  int        `json:"interval_seconds"`
	TimeoutMs        int        `json:"timeout_ms"`
	FailThreshold    int        `json:"fail_threshold"`
	Severity         string     `json:"severity"`
	ConsecutiveFails int        `json:"consecutive_fails"`
	ConsecutiveOKs   int        `json:"consecutive_oks"`
	LastStatus       string     `json:"last_status"`
	LastLatencyMs    *int       `json:"last_latency_ms"`
	LastError        string     `json:"last_error"`
	LastCheckedAt    *time.Time `json:"last_checked_at"`
	LastChangedAt    *time.Time `json:"last_changed_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type uptimeProbeInput struct {
	Name            string `json:"name"`
	Enabled         *bool  `json:"enabled"`
	Kind            string `json:"kind"`
	Target          string `json:"target"`
	Method          string `json:"method"`
	ExpectedStatus  *int   `json:"expected_status"`
	IntervalSeconds *int   `json:"interval_seconds"`
	TimeoutMs       *int   `json:"timeout_ms"`
	FailThreshold   *int   `json:"fail_threshold"`
	Severity        string `json:"severity"`
}

func (s *Server) handleListUptimeProbes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, enabled, kind, target, method, expected_status, interval_seconds, timeout_ms,
		       fail_threshold, severity, consecutive_fails, consecutive_oks, last_status, last_latency_ms,
		       last_error, last_checked_at, last_changed_at, created_at, updated_at
		FROM uptime_probes
		ORDER BY name
		LIMIT $1`, maxUptimeProbes)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list uptime probes.")
		return
	}
	defer rows.Close()
	out := make([]uptimeProbeRow, 0)
	for rows.Next() {
		var p uptimeProbeRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Enabled, &p.Kind, &p.Target, &p.Method, &p.ExpectedStatus,
			&p.IntervalSeconds, &p.TimeoutMs, &p.FailThreshold, &p.Severity, &p.ConsecutiveFails, &p.ConsecutiveOKs,
			&p.LastStatus, &p.LastLatencyMs, &p.LastError, &p.LastCheckedAt, &p.LastChangedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list uptime probes.")
			return
		}
		out = append(out, p)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{"total": len(out), "max": maxUptimeProbes},
	})
}

func (s *Server) handleCreateUptimeProbe(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	var body uptimeProbeInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid probe payload.")
		return
	}
	norm, errMsg := normalizeUptimeProbe(body, true)
	if errMsg != "" {
		httpx.Error(w, http.StatusBadRequest, "validation", errMsg)
		return
	}
	var count int
	if err := s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM uptime_probes`).Scan(&count); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create probe.")
		return
	}
	if count >= maxUptimeProbes {
		httpx.Error(w, http.StatusConflict, "limit", "Maximum of 20 uptime probes allowed.")
		return
	}

	var p uptimeProbeRow
	err := s.pool.QueryRow(r.Context(), `
		INSERT INTO uptime_probes (
			name, enabled, kind, target, method, expected_status, interval_seconds, timeout_ms, fail_threshold, severity
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, name, enabled, kind, target, method, expected_status, interval_seconds, timeout_ms,
		          fail_threshold, severity, consecutive_fails, consecutive_oks, last_status, last_latency_ms,
		          last_error, last_checked_at, last_changed_at, created_at, updated_at`,
		norm.Name, norm.Enabled, norm.Kind, norm.Target, norm.Method, norm.ExpectedStatus,
		norm.IntervalSeconds, norm.TimeoutMs, norm.FailThreshold, norm.Severity,
	).Scan(&p.ID, &p.Name, &p.Enabled, &p.Kind, &p.Target, &p.Method, &p.ExpectedStatus,
		&p.IntervalSeconds, &p.TimeoutMs, &p.FailThreshold, &p.Severity, &p.ConsecutiveFails, &p.ConsecutiveOKs,
		&p.LastStatus, &p.LastLatencyMs, &p.LastError, &p.LastCheckedAt, &p.LastChangedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create probe.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "uptime_probe.create", "uptime_probe", p.ID.String(), "ok", r.RemoteAddr, map[string]any{
		"name": p.Name, "kind": p.Kind, "target": p.Target,
	})
	httpx.JSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdateUptimeProbe(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid probe id.")
		return
	}
	var body uptimeProbeInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid probe payload.")
		return
	}
	norm, errMsg := normalizeUptimeProbe(body, false)
	if errMsg != "" {
		httpx.Error(w, http.StatusBadRequest, "validation", errMsg)
		return
	}
	var p uptimeProbeRow
	err = s.pool.QueryRow(r.Context(), `
		UPDATE uptime_probes SET
			name=$2, enabled=$3, kind=$4, target=$5, method=$6, expected_status=$7,
			interval_seconds=$8, timeout_ms=$9, fail_threshold=$10, severity=$11, updated_at=now()
		WHERE id=$1
		RETURNING id, name, enabled, kind, target, method, expected_status, interval_seconds, timeout_ms,
		          fail_threshold, severity, consecutive_fails, consecutive_oks, last_status, last_latency_ms,
		          last_error, last_checked_at, last_changed_at, created_at, updated_at`,
		id, norm.Name, norm.Enabled, norm.Kind, norm.Target, norm.Method, norm.ExpectedStatus,
		norm.IntervalSeconds, norm.TimeoutMs, norm.FailThreshold, norm.Severity,
	).Scan(&p.ID, &p.Name, &p.Enabled, &p.Kind, &p.Target, &p.Method, &p.ExpectedStatus,
		&p.IntervalSeconds, &p.TimeoutMs, &p.FailThreshold, &p.Severity, &p.ConsecutiveFails, &p.ConsecutiveOKs,
		&p.LastStatus, &p.LastLatencyMs, &p.LastError, &p.LastCheckedAt, &p.LastChangedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Probe not found.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "uptime_probe.update", "uptime_probe", p.ID.String(), "ok", r.RemoteAddr, map[string]any{
		"name": p.Name, "kind": p.Kind, "target": p.Target,
	})
	httpx.JSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteUptimeProbe(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid probe id.")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM uptime_probes WHERE id=$1`, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete probe.")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "Probe not found.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "uptime_probe.delete", "uptime_probe", id.String(), "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

type normalizedProbe struct {
	Name            string
	Enabled         bool
	Kind            string
	Target          string
	Method          string
	ExpectedStatus  int
	IntervalSeconds int
	TimeoutMs       int
	FailThreshold   int
	Severity        string
}

func normalizeUptimeProbe(in uptimeProbeInput, creating bool) (normalizedProbe, string) {
	out := normalizedProbe{
		Name:            strings.TrimSpace(in.Name),
		Enabled:         true,
		Kind:            strings.ToLower(strings.TrimSpace(in.Kind)),
		Target:          strings.TrimSpace(in.Target),
		Method:          strings.ToUpper(strings.TrimSpace(in.Method)),
		ExpectedStatus:  200,
		IntervalSeconds: 60,
		TimeoutMs:       3000,
		FailThreshold:   3,
		Severity:        "warning",
	}
	if in.Enabled != nil {
		out.Enabled = *in.Enabled
	}
	if out.Name == "" {
		return out, "Name is required."
	}
	if len(out.Name) > 80 {
		return out, "Name must be 80 characters or fewer."
	}
	if out.Kind != "http" && out.Kind != "tcp" {
		return out, "Kind must be http or tcp."
	}
	if out.Target == "" {
		return out, "Target is required."
	}
	if len(out.Target) > 500 {
		return out, "Target must be 500 characters or fewer."
	}
	if out.Kind == "http" {
		if out.Method == "" {
			out.Method = http.MethodGet
		}
		switch out.Method {
		case http.MethodGet, http.MethodHead:
		default:
			return out, "HTTP probes support GET or HEAD only."
		}
		u, err := url.Parse(out.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return out, "HTTP target must be an absolute http(s) URL."
		}
	} else {
		out.Method = "GET"
		if strings.Contains(out.Target, "://") || !strings.Contains(out.Target, ":") {
			return out, "TCP target must be host:port."
		}
	}
	if in.ExpectedStatus != nil {
		out.ExpectedStatus = *in.ExpectedStatus
	}
	if out.ExpectedStatus < 100 || out.ExpectedStatus > 599 {
		return out, "Expected status must be between 100 and 599."
	}
	if in.IntervalSeconds != nil {
		out.IntervalSeconds = *in.IntervalSeconds
	}
	if out.IntervalSeconds < 30 || out.IntervalSeconds > 600 {
		return out, "Interval must be between 30 and 600 seconds."
	}
	if in.TimeoutMs != nil {
		out.TimeoutMs = *in.TimeoutMs
	}
	if out.TimeoutMs < 500 || out.TimeoutMs > 10000 {
		return out, "Timeout must be between 500 and 10000 ms."
	}
	if in.FailThreshold != nil {
		out.FailThreshold = *in.FailThreshold
	}
	if out.FailThreshold < 1 || out.FailThreshold > 10 {
		return out, "Fail threshold must be between 1 and 10."
	}
	if sev := strings.ToLower(strings.TrimSpace(in.Severity)); sev != "" {
		out.Severity = sev
	}
	switch out.Severity {
	case "info", "warning", "critical":
	default:
		return out, "Severity must be info, warning, or critical."
	}
	_ = creating
	return out, ""
}
