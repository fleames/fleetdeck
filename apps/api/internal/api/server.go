package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/config"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/realtime"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg  config.Config
	pool *pgxpool.Pool
	auth *auth.Service
	hub  *realtime.Hub
}

func New(cfg config.Config, pool *pgxpool.Pool, hub *realtime.Hub) *Server {
	return &Server{cfg: cfg, pool: pool, auth: auth.NewService(pool, cfg.CookieSecure), hub: hub}
}

func (s *Server) Hub() *realtime.Hub { return s.hub }

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(s.secureHeaders)
	r.Use(s.requireCSRF)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.WebOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", s.handleHealthz)
	r.Get("/install.sh", s.handleInstallScript)
	r.Get("/install/agent/linux/{arch}", s.handleInstallAgentBinary)

	// WebSocket without request timeout middleware
	r.Get("/api/v1/realtime", s.handleRealtime)

	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(60 * time.Second))

		r.Route("/api/v1", func(r chi.Router) {
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/csrf", s.handleCSRF)
			r.Post("/auth/bootstrap", s.handleBootstrap)
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.requireUser(s.handleMe))

			r.Get("/overview", s.requireUser(s.handleOverview))
			r.Get("/servers", s.requireUser(s.handleListServers))
			r.Post("/servers", s.requireUser(s.handleCreateServer))
			r.Get("/servers/{id}", s.requireUser(s.handleGetServer))
			r.Post("/servers/{id}/remove", s.requireRole("admin", "operator")(s.handleRemoveServer))
			r.Post("/servers/{id}/update-agent", s.requireRole("admin", "operator")(s.handleUpdateAgent))
			r.Get("/servers/{id}/metrics", s.requireUser(s.handleServerLatestMetrics))
			r.Get("/servers/{id}/metrics/history", s.requireUser(s.handleServerMetricsHistory))
			r.Post("/servers/compare", s.requireUser(s.handleCompareServers))

			r.Get("/agents", s.requireUser(s.handleListAgents))
			r.Post("/agents/enrollment-tokens", s.requireRole("admin", "operator")(s.handleCreateEnrollmentToken))
			r.Post("/agents/install-oneline", s.requireRole("admin", "operator")(s.handleInstallOneline))
			r.Post("/agents/install-bundle", s.requireRole("admin", "operator")(s.handleInstallBundle))
			r.Post("/agents/install-command", s.requireRole("admin", "operator")(s.handleInstallCommand))

			r.Get("/alerts", s.requireUser(s.handleListAlerts))
			r.Get("/alert-rules", s.requireUser(s.handleListAlertRules))
			r.Post("/alerts/{id}/acknowledge", s.requireUser(s.handleAckAlert))
			r.Post("/alerts/{id}/resolve", s.requireUser(s.handleResolveAlert))
			r.Post("/alerts/{id}/silence", s.requireUser(s.handleSilenceAlert))

			r.Get("/events", s.requireUser(s.handleListEvents))
			r.Get("/search", s.requireUser(s.handleSearch))
			r.Get("/docker/summary", s.requireUser(s.handleDockerSummary))
			r.Get("/containers", s.requireUser(s.handleListContainers))
			r.Get("/containers/{id}", s.requireUser(s.handleGetContainer))
			r.Get("/containers/{id}/logs", s.requireUser(s.handleContainerLogs))
			r.Get("/containers/{id}/env", s.requireUser(s.handleContainerEnv))
			r.Post("/containers/{id}/actions/{action}", s.requireRole("admin", "operator")(s.handleContainerAction))
			r.Get("/images", s.requireUser(s.handleListImages))
			r.Get("/volumes", s.requireUser(s.handleListVolumes))
			r.Get("/networks", s.requireUser(s.handleListNetworks))
			r.Get("/compose", s.requireUser(s.handleListCompose))
			r.Get("/settings", s.requireUser(s.handleGetSettings))
			r.Patch("/settings", s.requireRole("admin")(s.handlePatchSettings))
			r.Get("/audit-logs", s.requireRole("admin")(s.handleListAudit))
			r.Get("/users", s.requireRole("admin")(s.handleListUsers))
			r.Post("/users", s.requireRole("admin")(s.handleCreateUser))
			r.Get("/export/{kind}", s.requireUser(s.handleExport))
			r.Get("/overview/self", s.requireUser(s.handleSelfMetrics))
			r.Get("/notifications", s.requireUser(s.handleNotifications))
			r.Post("/notifications/clear", s.requireUser(s.handleClearNotifications))
			r.Post("/notifications/{id}/dismiss", s.requireUser(s.handleDismissNotification))
			r.Get("/dashboard/layout", s.requireUser(s.handleGetDashboardLayout))
			r.Put("/dashboard/layout", s.requireUser(s.handlePutDashboardLayout))
			r.Post("/backup", s.requireRole("admin")(s.handleBackup))
			r.Post("/restore", s.requireRole("admin")(s.handleRestore))
		})

		r.Route("/agent/v1", func(r chi.Router) {
			r.With(maxBody(256 << 10)).Post("/enroll", s.handleAgentEnroll)
			r.With(maxBody(1<<20)).Post("/heartbeat", s.requireAgent(s.handleAgentHeartbeat))
			r.With(maxBody(8<<20)).Post("/metrics", s.requireAgent(s.handleAgentMetrics))
			r.With(maxBody(16<<20)).Post("/inventory", s.requireAgent(s.handleAgentInventory))
			r.Get("/commands", s.requireAgent(s.handleAgentPollCommands))
			r.With(maxBody(4<<20)).Post("/commands/{id}/result", s.requireAgent(s.handleAgentCommandResult))
		})
	})

	return r
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if s.cfg.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/realtime") {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKey string

const (
	ctxUser  ctxKey = "user"
	ctxAgent ctxKey = "agent"
)

func (s *Server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.auth.UserFromRequest(r.Context(), r)
		if err != nil {
			if errors.Is(err, auth.ErrUnauthenticated) {
				httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "Sign in required.")
				return
			}
			httpx.Error(w, http.StatusServiceUnavailable, "auth_unavailable",
				"Could not verify session. Retry shortly.")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser, u)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) optionalUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, err := s.auth.UserFromRequest(r.Context(), r); err == nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxUser, u))
		}
		next(w, r)
	}
}

type agentIdentity struct {
	AgentID  uuid.UUID
	ServerID uuid.UUID
}

func (s *Server) requireAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicID, secret, ok := parseAgentBearer(r.Header.Get("Authorization"))
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "agent_unauthenticated", "Agent credentials required.")
			return
		}
		var agentID, serverID uuid.UUID
		var secretHash string
		err := s.pool.QueryRow(r.Context(), `
			SELECT c.agent_id, a.server_id, c.secret_hash
			FROM agent_credentials c
			JOIN agents a ON a.id = c.agent_id
			WHERE c.public_id=$1 AND c.revoked_at IS NULL`, publicID,
		).Scan(&agentID, &serverID, &secretHash)
		if err != nil || !auth.VerifySecret(secretHash, secret) {
			httpx.Error(w, http.StatusUnauthorized, "agent_unauthenticated", "Invalid agent credentials.")
			return
		}
		ctx := context.WithValue(r.Context(), ctxAgent, agentIdentity{AgentID: agentID, ServerID: serverID})
		next(w, r.WithContext(ctx))
	}
}

func parseAgentBearer(h string) (publicID, secret string, ok bool) {
	const prefix = "Bearer "
	if len(h) < len(prefix) || h[:len(prefix)] != prefix {
		return "", "", false
	}
	raw := h[len(prefix):]
	for i := 0; i < len(raw); i++ {
		if raw[i] == ':' {
			return raw[:i], raw[i+1:], true
		}
	}
	return "", "", false
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		httpx.ErrorDetails(w, http.StatusServiceUnavailable, "database_unavailable",
			"FleetDeck cannot reach its database.", nil,
			map[string]any{"technical": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	ok := origin == "" || originAllowedAny(origin, s.cfg.WebOrigins)
	s.hub.ServeWS(w, r, ok)
}

func originAllowedAny(origin string, allowed []string) bool {
	for _, a := range allowed {
		if originAllowed(origin, a) {
			return true
		}
	}
	return false
}

func originAllowed(origin, allowed string) bool {
	if origin == allowed {
		return true
	}
	ou, err1 := url.Parse(origin)
	au, err2 := url.Parse(allowed)
	if err1 != nil || err2 != nil {
		return false
	}
	return ou.Scheme == au.Scheme && ou.Host == au.Host
}
