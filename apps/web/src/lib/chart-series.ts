/**
 * Insert nulls into a time series when consecutive points are farther apart
 * than maxGapMs so charts do not draw invented lines across missing data.
 */
export function withTimeGaps<T extends { ts: string | number | Date }>(
  points: T[],
  maxGapMs: number,
): Array<T | null> {
  if (points.length === 0) return [];
  const out: Array<T | null> = [points[0]];
  for (let i = 1; i < points.length; i++) {
    const prev = new Date(points[i - 1].ts).getTime();
    const cur = new Date(points[i].ts).getTime();
    if (Number.isFinite(prev) && Number.isFinite(cur) && cur - prev > maxGapMs) {
      out.push(null);
    }
    out.push(points[i]);
  }
  return out;
}

/** Expected max gap between samples for history source/range. */
export function historyGapMs(source?: string | null, range?: string | null): number {
  if (source === "1h" || range === "30d") return 2.5 * 60 * 60 * 1000;
  if (source === "5m" || range === "24h" || range === "7d") return 12.5 * 60 * 1000;
  // raw / short ranges — agent default interval ~10s
  return 45_000;
}
