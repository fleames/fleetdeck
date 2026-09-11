package collect

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestContainerActionAllowlist(t *testing.T) {
	ctx := context.Background()
	for _, action := range []string{"exec", "kill", "rm", "attach", "shell", ""} {
		err := ContainerAction(ctx, "abc123", action)
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("action %q: expected unsupported error, got %v", action, err)
		}
	}
}

func TestContainerActionPathResolve(t *testing.T) {
	cases := map[string]struct {
		method string
		path   string
	}{
		"start":   {http.MethodPost, "/containers/cid/start"},
		"STOP":    {http.MethodPost, "/containers/cid/stop?t=10"},
		"Restart": {http.MethodPost, "/containers/cid/restart?t=10"},
		"pause":   {http.MethodPost, "/containers/cid/pause"},
		"unpause": {http.MethodPost, "/containers/cid/unpause"},
		"remove":  {http.MethodDelete, "/containers/cid"},
	}
	for action, want := range cases {
		method, path, err := containerActionSpec("cid", action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if method != want.method || path != want.path {
			t.Fatalf("%s: method=%q path=%q want %q %q", action, method, path, want.method, want.path)
		}
	}
}
