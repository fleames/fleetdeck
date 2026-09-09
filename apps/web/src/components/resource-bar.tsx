export function ResourceBar({
  label,
  value,
}: {
  label: string;
  value: number | null;
}) {
  const display = value == null ? "—" : `${value.toFixed(0)}%`;
  const width = value == null ? 0 : Math.max(2, Math.min(100, value));
  const color =
    value == null
      ? "var(--border)"
      : value >= 90
        ? "var(--crit)"
        : value >= 75
          ? "var(--warn)"
          : "var(--ok)";
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-[11px] text-[var(--text-2)]">
        <span>{label}</span>
        <span className="font-[family-name:var(--font-mono-family)] text-[var(--text-1)]">
          {display}
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-[var(--bg-3)]">
        <div
          className="h-full rounded-full transition-all"
          style={{ width: `${width}%`, background: color }}
        />
      </div>
    </div>
  );
}
