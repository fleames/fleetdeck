package api

import "testing"

func TestIsSensitiveEnvKey(t *testing.T) {
	cases := map[string]bool{
		"PATH":            false,
		"NODE_ENV":        false,
		"DB_PASSWORD":     true,
		"API_TOKEN":       true,
		"AWS_SECRET_KEY":  true,
		"oauth_client_id": true, // contains AUTH substring
		"BASIC_AUTH_USER": true,
	}
	for k, want := range cases {
		if got := isSensitiveEnvKey(k); got != want {
			t.Fatalf("%s: got %v want %v", k, got, want)
		}
	}
}
