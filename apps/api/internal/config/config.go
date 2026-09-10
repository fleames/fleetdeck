package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIAddr              string
	APIPublicURL         string
	WebOrigin            string   // primary origin (first of WebOrigins)
	WebOrigins           []string // allowed browser origins (comma-separated WEB_ORIGIN)
	DatabaseURL          string
	SessionSecret        string
	MetricInterval       time.Duration
	RawRetentionDays     int
	Agg5mRetentionDays   int
	Agg1hRetentionDays   int
	AgentOfflineAfter    time.Duration
	CookieSecure         bool
	AgentDistDir         string
	AgentCDNBase         string
	AgentCDNChannel      string
	AgentVersion         string // expected agent release (AGENT_VERSION); matches apps/agent const
	RelayURL             string // wss://agents.tarkovbot.com/edge/connect — enables outbound edge
	RelayToken           string
	RelayLocalURL        string // loopback API for edge proxy (default http://127.0.0.1:8080)
	AlertWebhookURL      string // optional POST target on alert fire/resolve
}

func Load() (Config, error) {
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	webOrigins := parseOrigins(getenv("WEB_ORIGIN", "http://localhost:3000,http://127.0.0.1:3000"))
	cfg := Config{
		APIAddr:            getenv("API_ADDR", ":8080"),
		APIPublicURL:       getenv("API_PUBLIC_URL", "http://localhost:8080"),
		WebOrigin:          webOrigins[0],
		WebOrigins:         webOrigins,
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		SessionSecret:      os.Getenv("SESSION_SECRET"),
		MetricInterval:     time.Duration(getenvInt("DEFAULT_METRIC_INTERVAL_SECONDS", 10)) * time.Second,
		RawRetentionDays:   getenvInt("METRICS_RAW_RETENTION_DAYS", 7),
		Agg5mRetentionDays: getenvInt("METRICS_5M_RETENTION_DAYS", 30),
		Agg1hRetentionDays: getenvInt("METRICS_1H_RETENTION_DAYS", 365),
		AgentOfflineAfter:  time.Duration(getenvInt("AGENT_OFFLINE_AFTER_SECONDS", 45)) * time.Second,
		CookieSecure:       getenv("COOKIE_SECURE", "false") == "true",
		AgentDistDir:       getenv("AGENT_DIST_DIR", "/app/agent-dist"),
		AgentCDNBase:       strings.TrimRight(getenv("AGENT_CDN_BASE", "https://cdn.tarkovbot.com/fleetdeck"), "/"),
		AgentCDNChannel:    getenv("AGENT_CDN_CHANNEL", "latest"),
		AgentVersion:       getenv("AGENT_VERSION", "0.4.3-dev"),
		RelayURL:           strings.TrimSpace(os.Getenv("RELAY_URL")),
		RelayToken:         strings.TrimSpace(os.Getenv("RELAY_TOKEN")),
		RelayLocalURL:      getenv("RELAY_LOCAL_URL", "http://127.0.0.1:8080"),
		AlertWebhookURL:    strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_URL")),
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.SessionSecret) < 32 {
		return cfg, fmt.Errorf("SESSION_SECRET must be at least 32 characters")
	}
	return cfg, nil
}

func parseOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		o := strings.TrimRight(strings.TrimSpace(p), "/")
		if o == "" {
			continue
		}
		if _, ok := seen[o]; ok {
			continue
		}
		seen[o] = struct{}{}
		out = append(out, o)
	}
	if len(out) == 0 {
		return []string{"http://localhost:3000"}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
