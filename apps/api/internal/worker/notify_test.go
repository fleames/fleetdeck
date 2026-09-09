package worker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDetectWebhookFormat(t *testing.T) {
	cases := []struct {
		url, explicit, want string
	}{
		{"https://hooks.slack.com/services/T/B/X", "", "slack"},
		{"https://discord.com/api/webhooks/1/abc", "", "discord"},
		{"https://discordapp.com/api/webhooks/1/abc", "auto", "discord"},
		{"https://example.com/hook", "", "json"},
		{"https://hooks.slack.com/services/T/B/X", "json", "json"},
		{"https://example.com/hook", "discord", "discord"},
		{"https://example.com/hook", "slack", "slack"},
	}
	for _, c := range cases {
		got := DetectWebhookFormat(c.url, c.explicit)
		if got != c.want {
			t.Fatalf("url=%q explicit=%q got=%q want=%q", c.url, c.explicit, got, c.want)
		}
	}
}

func TestBuildWebhookPayloadDiscord(t *testing.T) {
	sid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	body, err := BuildWebhookPayload("discord", "alert.fired", "critical", "CPU high", &sid, map[string]any{"rule_id": "r1"}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["username"] != "FleetDeck" {
		t.Fatalf("username: %v", m["username"])
	}
	embeds, ok := m["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("embeds: %v", m["embeds"])
	}
}

func TestBuildWebhookPayloadSlack(t *testing.T) {
	body, err := BuildWebhookPayload("slack", "alert.resolved", "info", "back to normal", nil, nil, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	text, _ := m["text"].(string)
	if !strings.Contains(text, "alert.resolved") || !strings.Contains(text, "back to normal") {
		t.Fatalf("text=%q", text)
	}
}

func TestBuildWebhookPayloadJSON(t *testing.T) {
	sid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	body, err := BuildWebhookPayload("json", "alert.fired", "warning", "disk", &sid, map[string]any{"k": 1}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["source"] != "fleetdeck" || m["event"] != "alert.fired" {
		t.Fatalf("%v", m)
	}
	if m["server_id"] != sid.String() {
		t.Fatalf("server_id=%v", m["server_id"])
	}
}
