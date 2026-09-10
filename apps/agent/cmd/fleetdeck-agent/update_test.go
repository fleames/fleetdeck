package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchSHA256ForRequired(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/missing/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/ok/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  linux-amd64\n"))
	})
	mux.HandleFunc("/partial/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  linux-arm64\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := fetchSHA256For(srv.URL+"/missing/SHA256SUMS", "linux-amd64"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing SHA256SUMS error, got %v", err)
	}
	sum, err := fetchSHA256For(srv.URL+"/ok/SHA256SUMS", "linux-amd64")
	if err != nil || sum != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("sum=%q err=%v", sum, err)
	}
	if _, err := fetchSHA256For(srv.URL+"/partial/SHA256SUMS", "linux-amd64"); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("expected not listed error, got %v", err)
	}
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	payload := []byte("fleetdeck-agent-test")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(payload)
	want := hex.EncodeToString(h[:])
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestCmdlineMatchesAgent(t *testing.T) {
	bin := "/usr/local/bin/fleetdeck-agent"
	state := "/var/lib/fleetdeck"
	cases := []struct {
		name    string
		cmdline string
		want    bool
	}{
		{
			name:    "systemd fleetdeck user",
			cmdline: bin + "\x00-api\x00https://x\x00-state-dir\x00" + state + "\x00-interval\x0010s",
			want:    true,
		},
		{
			name:    "root manual same state",
			cmdline: bin + "\x00-state-dir\x00" + state,
			want:    true,
		},
		{
			name:    "equals form",
			cmdline: bin + "\x00-state-dir=" + state,
			want:    true,
		},
		{
			name:    "default state omitted",
			cmdline: bin + "\x00-api\x00https://x",
			want:    true,
		},
		{
			name:    "user install different path",
			cmdline: "/home/u/.local/bin/fleetdeck-agent\x00-state-dir\x00/home/u/.fleetdeck",
			want:    false,
		},
		{
			name:    "other binary",
			cmdline: "/usr/bin/docker\x00ps",
			want:    false,
		},
		{
			name:    "update oneshot same binary",
			cmdline: bin + "\x00-update",
			want:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmdlineMatchesAgent([]byte(tc.cmdline), bin, state)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
