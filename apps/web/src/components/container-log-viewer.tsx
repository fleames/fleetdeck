"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { apiFetch } from "@/lib/api";

function levelClass(line: string): string {
  const u = line.toUpperCase();
  if (u.includes("ERROR") || u.includes("FATAL") || u.includes("CRITICAL")) return "text-[var(--crit)]";
  if (u.includes("WARN")) return "text-[var(--warn)]";
  if (u.includes("DEBUG")) return "text-[var(--text-2)]";
  return "text-[var(--text-1)]";
}

function lineSeverity(line: string): "error" | "warn" | "info" | "debug" | "other" {
  const u = line.toUpperCase();
  if (u.includes("ERROR") || u.includes("FATAL") || u.includes("CRITICAL")) return "error";
  if (u.includes("WARN")) return "warn";
  if (u.includes("DEBUG")) return "debug";
  if (u.includes("INFO")) return "info";
  return "other";
}

function parseLineTs(line: string): number | null {
  const space = line.indexOf(" ");
  if (space <= 0 || space > 40) return null;
  const raw = line.slice(0, space);
  const ms = Date.parse(raw);
  return Number.isFinite(ms) ? Math.floor(ms / 1000) : null;
}

type SeverityFilter = "all" | "error" | "warn" | "info";

export function ContainerLogViewer({
  containerId,
  title = "Logs",
  maxHeightClass = "max-h-[560px]",
}: {
  containerId: string;
  title?: string;
  maxHeightClass?: string;
}) {
  const [lines, setLines] = useState<string[]>([]);
  const [filter, setFilter] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [severity, setSeverity] = useState<SeverityFilter>("all");
  const [tail, setTail] = useState(300);
  const [follow, setFollow] = useState(false);
  const [paused, setPaused] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [wrap, setWrap] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastFetch, setLastFetch] = useState<string | null>(null);
  const sinceRef = useRef<number>(0);
  const preRef = useRef<HTMLPreElement | null>(null);
  const seenRef = useRef<Set<string>>(new Set());

  const fetchLogs = useCallback(
    async (opts?: { incremental?: boolean }) => {
      if (!containerId) return;
      setBusy(true);
      setError(null);
      try {
        const incremental = Boolean(opts?.incremental && sinceRef.current > 0);
        const params = new URLSearchParams();
        params.set("tail", String(incremental ? Math.min(tail, 500) : tail));
        if (incremental) {
          // Re-fetch slightly overlapping window so we do not miss boundary lines.
          params.set("since", String(Math.max(0, sinceRef.current - 1)));
        }
        const res = await apiFetch(`/api/v1/containers/${containerId}/logs?${params}`);
        const data = await res.json();
        if (!res.ok) throw new Error(data?.error?.message ?? "Could not fetch logs");
        const next = (data.lines ?? []) as string[];
        setLastFetch(new Date().toISOString());

        if (!incremental) {
          seenRef.current = new Set(next);
          setLines(next);
        } else if (next.length > 0) {
          setLines((prev) => {
            const merged = [...prev];
            for (const line of next) {
              if (seenRef.current.has(line)) continue;
              seenRef.current.add(line);
              merged.push(line);
            }
            // Cap retained buffer for follow mode.
            if (merged.length > 5000) {
              const trimmed = merged.slice(merged.length - 5000);
              seenRef.current = new Set(trimmed);
              return trimmed;
            }
            return merged;
          });
        }

        let maxTs = sinceRef.current;
        for (const line of next) {
          const ts = parseLineTs(line);
          if (ts != null && ts > maxTs) maxTs = ts;
        }
        if (maxTs > 0) sinceRef.current = maxTs;
        else if (!incremental) sinceRef.current = Math.floor(Date.now() / 1000);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Log fetch failed");
      } finally {
        setBusy(false);
      }
    },
    [containerId, tail],
  );

  useEffect(() => {
    if (!follow || paused || !containerId) return;
    const boot = window.setTimeout(() => void fetchLogs({ incremental: true }), 0);
    const id = window.setInterval(() => void fetchLogs({ incremental: true }), 3000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(id);
    };
  }, [follow, paused, containerId, fetchLogs]);

  useEffect(() => {
    if (!autoScroll || !follow || !preRef.current) return;
    preRef.current.scrollTop = preRef.current.scrollHeight;
  }, [lines, autoScroll, follow]);

  const filtered = useMemo(() => {
    return lines.filter((line) => {
      if (severity !== "all") {
        const sev = lineSeverity(line);
        if (severity === "error" && sev !== "error") return false;
        if (severity === "warn" && sev !== "warn" && sev !== "error") return false;
        if (severity === "info" && sev === "debug") return false;
      }
      if (!filter) return true;
      if (caseSensitive) return line.includes(filter);
      return line.toLowerCase().includes(filter.toLowerCase());
    });
  }, [lines, filter, caseSensitive, severity]);

  return (
    <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
      <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">{title}</h2>
          <p className="text-xs text-[var(--text-2)]">
            Plain-text logs via the host agent. Follow polls every 3s
            {lastFetch ? ` · last ${new Date(lastFetch).toLocaleTimeString()}` : ""}.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-[11px] text-[var(--text-2)]">
            Tail
            <select
              className="ml-1 rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-2 py-1 text-xs"
              value={tail}
              onChange={(e) => setTail(Number(e.target.value))}
            >
              {[100, 300, 500, 1000, 2000].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter lines…"
            className="min-w-[140px] rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-2 py-1 text-xs"
          />
          <label className="flex items-center gap-1 text-[11px] text-[var(--text-2)]">
            <input
              type="checkbox"
              checked={caseSensitive}
              onChange={(e) => setCaseSensitive(e.target.checked)}
            />
            Aa
          </label>
          <select
            className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-2 py-1 text-xs"
            value={severity}
            onChange={(e) => setSeverity(e.target.value as SeverityFilter)}
          >
            <option value="all">All levels</option>
            <option value="error">Errors</option>
            <option value="warn">Warnings+</option>
            <option value="info">Info+</option>
          </select>
          <button
            type="button"
            className="rounded-md border border-[var(--border)] px-2 py-1 text-xs"
            onClick={() => setWrap((v) => !v)}
          >
            {wrap ? "Unwrap" : "Wrap"}
          </button>
          <button
            type="button"
            className={`rounded-md border px-2 py-1 text-xs ${
              follow
                ? "border-[var(--accent)] bg-[var(--accent)]/15 text-[var(--accent)]"
                : "border-[var(--border)]"
            }`}
            onClick={() => {
              setFollow((v) => {
                const next = !v;
                if (next) {
                  setPaused(false);
                  void fetchLogs({ incremental: lines.length > 0 });
                }
                return next;
              });
            }}
          >
            {follow ? "Following" : "Follow"}
          </button>
          {follow && (
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-2 py-1 text-xs"
              onClick={() => setPaused((v) => !v)}
            >
              {paused ? "Resume" : "Pause"}
            </button>
          )}
          <label className="flex items-center gap-1 text-[11px] text-[var(--text-2)]">
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(e) => setAutoScroll(e.target.checked)}
            />
            Autoscroll
          </label>
          <button
            type="button"
            className="rounded-md border border-[var(--border)] px-2 py-1 text-xs"
            onClick={() => {
              setLines([]);
              seenRef.current = new Set();
              sinceRef.current = 0;
            }}
          >
            Clear
          </button>
          <button
            type="button"
            disabled={!containerId || busy}
            className="rounded-md bg-[var(--accent)] px-3 py-1 text-xs text-white disabled:opacity-60"
            onClick={() => {
              sinceRef.current = 0;
              void fetchLogs({ incremental: false });
            }}
          >
            {busy ? "Fetching…" : "Fetch"}
          </button>
        </div>
      </div>

      {error && (
        <div className="mb-3 rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      <pre
        ref={preRef}
        className={`${maxHeightClass} overflow-auto rounded-md bg-[var(--bg-0)] p-3 font-[family-name:var(--font-mono-family)] text-xs ${
          wrap ? "whitespace-pre-wrap break-words" : "whitespace-pre"
        }`}
      >
        {filtered.length === 0
          ? "No log lines loaded. Fetch once, or enable Follow to poll the agent."
          : filtered.map((line, i) => (
              <div key={`${i}-${line.slice(0, 24)}`} className={levelClass(line)}>
                {line}
              </div>
            ))}
      </pre>
      <p className="mt-2 text-[11px] text-[var(--text-2)]">
        Showing {filtered.length} of {lines.length} lines. HTML/JS from containers cannot execute here.
      </p>
    </section>
  );
}
