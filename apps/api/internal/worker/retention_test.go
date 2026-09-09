package worker

import "testing"

func TestApplyMetricsSettingsJSON(t *testing.T) {
	raw, a5, a1 := applyMetricsSettingsJSON([]byte(`{"raw_retention_days":14,"agg_5m_retention_days":60,"agg_1h_retention_days":400}`), 7, 30, 365)
	if raw != 14 || a5 != 60 || a1 != 400 {
		t.Fatalf("got %d %d %d", raw, a5, a1)
	}
	raw, a5, a1 = applyMetricsSettingsJSON([]byte(`{"raw_retention_days":0}`), 7, 30, 365)
	if raw != 7 {
		t.Fatalf("zero should not override, got %d", raw)
	}
	raw, a5, a1 = applyMetricsSettingsJSON([]byte(`not-json`), 7, 30, 365)
	if raw != 7 || a5 != 30 || a1 != 365 {
		t.Fatalf("invalid json should keep defaults")
	}
}

func TestNormalizeRetentionDays(t *testing.T) {
	r, a, h := normalizeRetentionDays(0, -1, 0)
	if r != 7 || a != 30 || h != 365 {
		t.Fatalf("got %d %d %d", r, a, h)
	}
}
