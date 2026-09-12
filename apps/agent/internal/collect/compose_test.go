package collect

import (
	"strings"
	"testing"
)

func TestComposeActionAllowlist(t *testing.T) {
	for _, action := range []string{"exec", "build", "kill", ""} {
		err := ComposeAction(t.Context(), "demo", action)
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("action %q: expected unsupported, got %v", action, err)
		}
	}
}

func TestComposeActionRequiresProject(t *testing.T) {
	err := ComposeAction(t.Context(), "", "start")
	if err == nil || !strings.Contains(err.Error(), "project_name") {
		t.Fatalf("expected project_name error, got %v", err)
	}
}

func TestSplitImageRef(t *testing.T) {
	cases := map[string][2]string{
		"nginx:alpine":                   {"nginx", "alpine"},
		"ghcr.io/org/app:1.2":            {"ghcr.io/org/app", "1.2"},
		"localhost:5000/app:latest":      {"localhost:5000/app", "latest"},
		"nginx":                          {"nginx", "latest"},
		"nginx@sha256:abcd":              {"nginx@sha256:abcd", ""},
	}
	for in, want := range cases {
		image, tag := splitImageRef(in)
		if image != want[0] || tag != want[1] {
			t.Fatalf("%s => %s %s, want %s %s", in, image, tag, want[0], want[1])
		}
	}
}
