export default function PlaceholderPage({
  title,
  summary,
}: {
  title: string;
  summary: string;
}) {
  return (
    <div className="mx-auto max-w-3xl">
      <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
        Coming online
      </div>
      <h1 className="mt-1 text-3xl font-semibold tracking-tight">{title}</h1>
      <p className="mt-2 text-sm text-[var(--text-1)]">{summary}</p>
      <div className="mt-6 rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] bg-[var(--bg-1)] px-5 py-8 text-sm text-[var(--text-2)]">
        This surface is scaffolded and intentionally empty — no fake metrics or simulated Docker data.
        It will light up from real agent reports in later phases.
      </div>
    </div>
  );
}
