package collect

import (
	"context"
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
	cases := map[string]string{
		"start":   "/containers/cid/start",
		"STOP":    "/containers/cid/stop?t=10",
		"Restart": "/containers/cid/restart?t=10",
		"pause":   "/containers/cid/pause",
		"unpause": "/containers/cid/unpause",
	}
	for action, want := range cases {
		got, err := containerActionPath("cid", action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if got != want {
			t.Fatalf("%s: path=%q want %q", action, got, want)
		}
	}
}
