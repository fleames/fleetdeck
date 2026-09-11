package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentCommandWait(t *testing.T) {
	tests := []struct {
		query string
		want  time.Duration
	}{
		{"", 0},
		{"?wait=25s", 25 * time.Second},
		{"?wait=90s", 30 * time.Second},
		{"?wait=-1s", 0},
		{"?wait=invalid", 0},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/agent/v1/commands"+tt.query, nil)
		if got := agentCommandWait(req); got != tt.want {
			t.Errorf("query %q: got %s, want %s", tt.query, got, tt.want)
		}
	}
}
