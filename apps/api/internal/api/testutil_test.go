package api

import (
	"context"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/config"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/google/uuid"
)

func testCfg() config.Config {
	return config.Config{
		APIAddr:       ":8080",
		WebOrigin:     "http://localhost:3000",
		WebOrigins:    []string{"http://localhost:3000", "http://127.0.0.1:3000"},
		CookieSecure:  false,
		SessionSecret: "dev-only-replace-with-openssl-rand-base64-48-chars-min",
	}
}

func testAdmin() auth.User {
	return auth.User{
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Email:       "admin@test.local",
		DisplayName: "Admin",
		Role:        "admin",
	}
}

func withUser(ctx context.Context, u auth.User) context.Context {
	return context.WithValue(ctx, ctxUser, u)
}
