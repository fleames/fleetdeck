package collect

import "testing"

func TestParseLogLineTimestamp(t *testing.T) {
	line := "2026-09-12T17:00:01.123456789Z hello world"
	ts, ok := ParseLogLineTimestamp(line)
	if !ok {
		t.Fatal("expected timestamp")
	}
	if ts.Year() != 2026 || ts.Month() != 9 || ts.Day() != 12 {
		t.Fatalf("unexpected ts %v", ts)
	}
	if _, ok := ParseLogLineTimestamp("not a timestamp line"); ok {
		t.Fatal("expected no timestamp")
	}
}
