import type { ReactNode } from "react";

export function ErrorBanner({ message }: { message: string }) {
  return (
    <div
      role="alert"
      className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]"
    >
      {message}
    </div>
  );
}

export function LoadingBlock({ label = "Loading…" }: { label?: string }) {
  return (
    <div
      className="h-40 animate-pulse rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-2)]"
      aria-busy="true"
      aria-label={label}
    />
  );
}

export function EmptyBlock({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-6 py-12 text-center text-sm text-[var(--text-1)]">
      {children}
    </div>
  );
}
