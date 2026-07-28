import type { Project } from '../../types';

// One row of the comparison matrix: an item (feature or skill) and the set of
// projects where it is effectively enabled.
export interface MatrixRow {
  id: number;
  name: string;
  // Small muted line under the name (e.g. a skill category or a feature's skills).
  subtitle?: string;
  enabledProjectIds: Set<number>;
  // Display-only cells (no toggling) — e.g. a project-owned skill fork that only
  // exists in one project. `lockedHint` becomes the cell tooltip.
  locked?: boolean;
  lockedHint?: string;
}

const CheckIcon = () => (
  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M20 6 9 17l-5-5" />
  </svg>
);

const TimesIcon = () => (
  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M6 6l12 12M18 6L6 18" />
  </svg>
);

// A features/skills × projects grid. Each cell shows a check (enabled) or a
// times (disabled) icon; clicking a cell toggles that item for that single
// project (unless the row is locked). The first column and header stay pinned so
// the grid stays readable as it scrolls horizontally across many projects.
export function ProjectMatrixTable({
  itemLabel,
  rows,
  projects,
  busyKey,
  onToggle,
  emptyLabel = 'Nothing to compare yet.',
}: {
  itemLabel: string;
  rows: MatrixRow[];
  projects: Project[];
  busyKey: string | null;
  onToggle: (row: MatrixRow, project: Project, enabled: boolean) => void;
  emptyLabel?: string;
}) {
  const total = projects.length;

  if (rows.length === 0 || total === 0) {
    return (
      <p className="rounded-2xl border border-gray-200 bg-white px-4 py-6 text-sm text-gray-500 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-400">
        {total === 0 ? 'No projects yet.' : emptyLabel}
      </p>
    );
  }

  return (
    <div className="overflow-x-auto rounded-2xl border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-gray-200 dark:border-gray-700">
            <th className="sticky left-0 z-10 bg-white px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-gray-800 dark:text-gray-400">
              {itemLabel}
            </th>
            {projects.map((p) => (
              <th
                key={p.id}
                className="whitespace-nowrap px-3 py-3 text-center text-xs font-semibold text-gray-600 dark:text-gray-300"
                title={p.name}
              >
                {p.name}
              </th>
            ))}
            <th className="px-3 py-3 text-center text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Enabled
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100 dark:divide-gray-700/60">
          {rows.map((row) => {
            const onCount = projects.reduce(
              (n, p) => n + (row.enabledProjectIds.has(p.id) ? 1 : 0),
              0,
            );
            return (
              <tr key={row.id} className="group">
                <th
                  scope="row"
                  className="sticky left-0 z-10 bg-white px-4 py-3 text-left align-middle font-normal group-hover:bg-gray-50 dark:bg-gray-800 dark:group-hover:bg-gray-700/40"
                >
                  <div className="min-w-40 max-w-64">
                    <div className="truncate text-sm font-medium text-gray-900 dark:text-gray-50">
                      {row.name}
                    </div>
                    {row.subtitle && (
                      <div className="truncate text-xs text-gray-400 dark:text-gray-500">
                        {row.subtitle}
                      </div>
                    )}
                  </div>
                </th>
                {projects.map((p) => {
                  const on = row.enabledProjectIds.has(p.id);
                  const busy = busyKey === `${row.id}:${p.id}`;
                  const cellInner = on ? <CheckIcon /> : <TimesIcon />;
                  if (row.locked) {
                    return (
                      <td key={p.id} className="px-3 py-3 text-center">
                        <span
                          title={row.lockedHint}
                          className={`inline-flex h-8 w-8 items-center justify-center rounded-lg ${
                            on
                              ? 'text-emerald-600 dark:text-emerald-400'
                              : 'text-gray-300 dark:text-gray-600'
                          }`}
                        >
                          {cellInner}
                        </span>
                      </td>
                    );
                  }
                  return (
                    <td key={p.id} className="px-3 py-3 text-center">
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => onToggle(row, p, !on)}
                        title={
                          on
                            ? `Enabled in ${p.name} — click to disable`
                            : `Disabled in ${p.name} — click to enable`
                        }
                        className={`inline-flex h-8 w-8 items-center justify-center rounded-lg border transition disabled:opacity-40 ${
                          on
                            ? 'border-emerald-200 bg-emerald-50 text-emerald-600 hover:bg-emerald-100 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-400 dark:hover:bg-emerald-500/20'
                            : 'border-gray-200 bg-white text-gray-300 hover:border-gray-300 hover:text-gray-500 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-600 dark:hover:border-gray-600 dark:hover:text-gray-400'
                        }`}
                      >
                        {cellInner}
                      </button>
                    </td>
                  );
                })}
                <td className="px-3 py-3 text-center">
                  <span
                    className={`inline-flex min-w-10 items-center justify-center rounded-full px-2 py-0.5 text-xs font-semibold ${
                      onCount === 0
                        ? 'bg-gray-100 text-gray-400 dark:bg-gray-700 dark:text-gray-500'
                        : 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
                    }`}
                  >
                    {onCount}/{total}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
