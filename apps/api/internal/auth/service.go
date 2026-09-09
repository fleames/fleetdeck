package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SessionCookie = "fleetdeck_session"

// ErrUnauthenticated means no valid session cookie (missing, revoked, or expired).
// Other errors from UserFromRequest are infrastructure failures and must not be treated as logout.
var ErrUnauthenticated = errors.New("unauthenticated")

type User struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
}

type Service struct {
	pool          *pgxpool.Pool
	cookieSecure  bool
	sessionTTL    time.Duration
}

func NewService(pool *pgxpool.Pool, cookieSecure bool) *Service {
	return &Service{pool: pool, cookieSecure: cookieSecure, sessionTTL: 7 * 24 * time.Hour}
}

func (s *Service) NeedsBootstrap(ctx context.Context) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n == 0, err
}

func (s *Service) Bootstrap(ctx context.Context, email, password, displayName string) (User, error) {
	needs, err := s.NeedsBootstrap(ctx)
	if err != nil {
		return User{}, err
	}
	if !needs {
		return User{}, errors.New("bootstrap already completed")
	}
	if len(password) < 12 {
		return User{}, errors.New("password must be at least 12 characters")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (email, display_name, password_hash, role)
		VALUES ($1, $2, $3, 'admin')
		RETURNING id, email, display_name, role`,
		email, displayName, hash,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	return u, err
}

func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (User, string, error) {
	var u User
	var hash string
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, display_name, role, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, "", errors.New("invalid credentials")
		}
		return User{}, "", err
	}
	if !VerifyPassword(hash, password) {
		return User{}, "", errors.New("invalid credentials")
	}
	token, err := NewToken(32)
	if err != nil {
		return User{}, "", err
	}
	expires := time.Now().UTC().Add(s.sessionTTL)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5)`,
		u.ID, HashToken(token), expires, ip, ua,
	)
	if err != nil {
		return User{}, "", err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE users SET last_login_at=now() WHERE id=$1`, u.ID)
	return u, token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL`, HashToken(token))
	return err
}

func (s *Service) UserFromRequest(ctx context.Context, r *http.Request) (User, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return User{}, ErrUnauthenticated
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()`, HashToken(c.Value),
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUnauthenticated
		}
		return User{}, err
	}
	return u, nil
}

func (s *Service) SetSessionCookie(w http.ResponseWriter, token string) {
	// Path=/ + SameSite=Lax works for localhost cross-port (3000→8080) same-site fetches.
	// Secure follows COOKIE_SECURE (false for local http; true behind HTTPS).
	// No Domain attribute — host-only cookie for the API host avoids localhost/127.0.0.1 mismatches.
	maxAge := int(s.sessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
		MaxAge:   maxAge,
		Expires:  time.Now().UTC().Add(s.sessionTTL),
	})
}

func (s *Service) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

func (s *Service) Audit(ctx context.Context, userID *uuid.UUID, action, targetType, targetID, result, ip string, context map[string]any) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, target_type, target_id, result, ip, context)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7::jsonb, '{}'::jsonb))`,
		userID, action, targetType, targetID, result, ip, mustJSON(context),
	)
	if err != nil {
		log.Printf("CRITICAL: audit_logs insert failed action=%s target=%s/%s result=%s: %v",
			action, targetType, targetID, result, err)
	}
	return err
}
