package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/google/uuid"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, email, display_name, role, created_at FROM users ORDER BY email`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list users.")
		return
	}
	defer rows.Close()
	type row struct {
		ID          uuid.UUID `json:"id"`
		Email       string    `json:"email"`
		DisplayName string    `json:"display_name"`
		Role        string    `json:"role"`
		CreatedAt   time.Time `json:"created_at"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var u row
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list users.")
			return
		}
		out = append(out, u)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value(ctxUser).(auth.User)
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	body.Role = strings.ToLower(strings.TrimSpace(body.Role))
	if body.Email == "" || body.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Email and password are required.")
		return
	}
	if len(body.Password) < 12 {
		httpx.Error(w, http.StatusBadRequest, "validation", "Password must be at least 12 characters.")
		return
	}
	if body.Role != "admin" && body.Role != "operator" && body.Role != "viewer" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Role must be admin, operator, or viewer.")
		return
	}
	if body.DisplayName == "" {
		body.DisplayName = body.Email
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create user.")
		return
	}
	var u auth.User
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO users (email, display_name, password_hash, role)
		VALUES ($1,$2,$3,$4)
		RETURNING id, email, display_name, role`,
		body.Email, body.DisplayName, hash, body.Role,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "conflict", "Could not create user (email may already exist).")
		return
	}
	uid := actor.ID
	s.auth.Audit(r.Context(), &uid, "user.create", "user", u.ID.String(), "ok", r.RemoteAddr, map[string]any{
		"email": u.Email, "role": u.Role,
	})
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": u})
}
