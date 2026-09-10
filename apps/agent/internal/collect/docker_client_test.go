package collect

import "testing"

func TestDockerHTTPClientShared(t *testing.T) {
	a := dockerHTTPClient()
	b := dockerHTTPClient()
	if a == nil || b == nil {
		t.Fatal("expected non-nil client")
	}
	if a != b {
		t.Fatal("dockerHTTPClient must return a process-wide shared *http.Client")
	}
}
