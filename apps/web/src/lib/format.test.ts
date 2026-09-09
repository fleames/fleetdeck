import { test } from "node:test";
import assert from "node:assert/strict";
import { freshnessLabel } from "./format.ts";
import { historyGapMs, withTimeGaps } from "./chart-series.ts";

test("freshnessLabel LIVE RECENT STALE OFFLINE", () => {
  const now = Date.parse("2026-09-09T12:00:00Z");
  assert.equal(freshnessLabel(null, "offline", now).code, "OFFLINE");
  assert.equal(freshnessLabel(null, "online", now).code, "NONE");
  assert.equal(freshnessLabel("2026-09-09T11:59:50Z", "online", now).code, "LIVE");
  assert.equal(freshnessLabel("2026-09-09T11:58:30Z", "online", now).code, "RECENT");
  assert.equal(freshnessLabel("2026-09-09T11:50:00Z", "online", now).code, "STALE");
  assert.match(freshnessLabel("2026-09-09T11:59:50Z", "online", now).label, /^LIVE/);
});

test("withTimeGaps inserts null across missing windows", () => {
  const pts = [
    { ts: "2026-09-09T12:00:00Z", v: 1 },
    { ts: "2026-09-09T12:00:10Z", v: 2 },
    { ts: "2026-09-09T12:05:00Z", v: 3 },
  ];
  const gapped = withTimeGaps(pts, 45_000);
  assert.equal(gapped.length, 4);
  assert.equal(gapped[2], null);
  assert.equal(gapped[3]?.v, 3);
});

test("historyGapMs by source", () => {
  assert.equal(historyGapMs("raw", "1h"), 45_000);
  assert.equal(historyGapMs("5m", "24h"), 12.5 * 60 * 1000);
  assert.equal(historyGapMs("1h", "30d"), 2.5 * 60 * 60 * 1000);
});
