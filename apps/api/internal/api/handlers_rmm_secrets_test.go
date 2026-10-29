package api

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFormatLocalRDPUsername(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"fleetdeck":      `.\fleetdeck`,
		`.\fleetdeck`:    `.\fleetdeck`,
		`HOST\fleetdeck`: `HOST\fleetdeck`,
		"  fleetdeck  ":  `.\fleetdeck`,
		`DOMAIN/user`:    `DOMAIN/user`,
	}
	for in, want := range cases {
		if got := formatLocalRDPUsername(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestLocalRDPAccountParts(t *testing.T) {
	cases := []struct {
		in, user, domain string
	}{
		{"", "", ""},
		{"fleetdeck", "fleetdeck", "."},
		{`.\fleetdeck`, "fleetdeck", "."},
		{`HOST\fleetdeck`, "fleetdeck", "HOST"},
		{"DOMAIN/user", "user", "DOMAIN"},
	}
	for _, c := range cases {
		u, d := localRDPAccountParts(c.in)
		if u != c.user || d != c.domain {
			t.Fatalf("%q: got (%q,%q) want (%q,%q)", c.in, u, d, c.user, c.domain)
		}
	}
}

func TestValidCDNChannel(t *testing.T) {
	ok := []string{"latest", "stable", "v1.2.3", "release_candidate"}
	bad := []string{"", "../evil", "a/b", "chan nel", strings.Repeat("x", 65), "bad;drop"}
	for _, c := range ok {
		if !validCDNChannel(c) {
			t.Fatalf("expected ok: %q", c)
		}
	}
	for _, c := range bad {
		if validCDNChannel(c) {
			t.Fatalf("expected reject: %q", c)
		}
	}
}

func TestValidLocalRDPUser(t *testing.T) {
	if !validLocalRDPUser("fleetdeck") {
		t.Fatal("fleetdeck should be valid")
	}
	if validLocalRDPUser(`adm';n`) || validLocalRDPUser("a b") || validLocalRDPUser("") {
		t.Fatal("expected reject")
	}
}

func TestSanitizeGuacDisplayParamsDropsTargets(t *testing.T) {
	q := url.Values{}
	q.Set("hostname", "evil.example")
	q.Set("port", "22")
	q.Set("password", "secret")
	q.Set("username", "root")
	q.Set("width", "1024")
	q.Set("enable-drive", "true")
	q.Set("enable-sftp", "true")
	out := sanitizeGuacDisplayParams(q)
	if out.Get("hostname") != "" || out.Get("port") != "" || out.Get("password") != "" || out.Get("username") != "" {
		t.Fatalf("target/cred fields leaked: %v", out)
	}
	if out.Get("width") != "1024" {
		t.Fatalf("width: %q", out.Get("width"))
	}
	if out.Get("enable-drive") != "false" {
		t.Fatalf("enable-drive should be forced false")
	}
}

func TestApplyRMMRDPCredentialsIgnoresClientPassword(t *testing.T) {
	s := &Server{}
	sessionID := uuid.New()
	hostID := uuid.New()
	s.setRMMSessionCredOverride(sessionID, "fleetdeck", "panel-secret")

	q := url.Values{}
	q.Set("hostname", "should-keep")
	q.Set("port", "12345")
	q.Set("password", "attacker")
	q.Set("username", "attacker")
	q.Set("override_credentials", "true")

	s.applyRMMRDPCredentials(context.Background(), hostID, sessionID, q)
	if q.Get("password") != "panel-secret" {
		t.Fatalf("password=%q want panel-secret", q.Get("password"))
	}
	if q.Get("username") != "fleetdeck" {
		t.Fatalf("username=%q", q.Get("username"))
	}
	if q.Get("hostname") != "should-keep" || q.Get("port") != "12345" {
		t.Fatalf("hostname/port must be preserved: %v", q)
	}
}

func TestSessionCredOverrideExpiry(t *testing.T) {
	s := &Server{}
	id := uuid.New()
	rmmCredOverrideMu.Lock()
	rmmCredOverrides[id] = rmmSessionCredOverride{
		Username: "u",
		Password: "p",
		Expires:  time.Now().UTC().Add(-time.Minute),
	}
	rmmCredOverrideMu.Unlock()
	if _, ok := s.takeRMMSessionCredOverride(id); ok {
		t.Fatal("expired override should be rejected")
	}
}
