package api

import "testing"

func TestValidSecretKindName(t *testing.T) {
	if !validSecretKindName("webhook", "alert_signing") {
		t.Fatal("expected webhook/alert_signing valid")
	}
	if validSecretKindName("arbitrary", "alert_signing") {
		t.Fatal("arbitrary kind should be rejected")
	}
	if validSecretKindName("webhook", "Bad-Name") {
		t.Fatal("invalid name should be rejected")
	}
	if validSecretKindName("webhook", "") {
		t.Fatal("empty name should be rejected")
	}
}
