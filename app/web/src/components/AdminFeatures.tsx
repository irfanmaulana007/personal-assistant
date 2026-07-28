import { useEffect, useMemo, useState } from 'react';
import { listAdminFeatures, setProjectFeature } from '../api/client';
import type { AdminFeature, Project } from '../types';
import { Skeleton, SkeletonListRow } from './ui/Skeleton';
import { SegmentedTabs } from './ui/SegmentedTabs';
import { ProjectMatrixTable, type MatrixRow } from './ui/ProjectMatrixTable';
import { matrixRowMatches } from '../lib/matrix';
import { useProjects } from '../contexts/project';

type ViewKey = 'catalog' | 'comparison';

// The platform-wide features surface (superadmin). "Catalog" is the master list
// of features and the skills they own; "Projects comparison" is a features ×
// projects matrix whose check/times cells toggle a feature on or off per project.
export function AdminFeatures() {
  const { projects } = useProjects();
  const [features, setFeatures] = useState<AdminFeature[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const [view, setView] = useState<ViewKey>('catalog');
  const [busyKey, setBusyKey] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    listAdminFeatures()
      .then((f) => {
        if (active) setFeatures(f);
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load features');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return features.filter(
      (f) =>
        !q ||
        `${f.name} ${f.description} ${f.key} ${f.skill_keys.join(' ')}`.toLowerCase().includes(q),
    );
  }, [features, query]);

  // Rows for the comparison matrix; the subtitle lists the skills a feature owns.
  const rows: MatrixRow[] = useMemo(
    () =>
      visible.map((f) => ({
        id: f.id,
        name: f.name,
        subtitle: f.skill_keys.length ? `Skills: ${f.skill_keys.join(', ')}` : undefined,
        enabledProjectIds: new Set(f.projects.map((p) => p.id)),
      })),
    [visible],
  );

  // Enable/disable a feature for a single project. The path-scoped endpoint
  // returns that project's feature list (not the admin matrix), so we
  // optimistically fold the change into the feature's `projects` array.
  const toggle = async (row: MatrixRow, project: Project, enabled: boolean) => {
    setBusyKey(`${row.id}:${project.id}`);
    setError('');
    try {
      await setProjectFeature(project.id, row.id, enabled);
      setFeatures((prev) =>
        prev.map((f) => {
          if (f.id !== row.id) return f;
          const has = f.projects.some((p) => p.id === project.id);
          if (enabled && !has) {
            return {
              ...f,
              projects: [...f.projects, { id: project.id, name: project.name, slug: project.slug }],
            };
          }
          if (!enabled && has) {
            return { ...f, projects: f.projects.filter((p) => p.id !== project.id) };
          }
          return f;
        }),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update feature');
    } finally {
      setBusyKey(null);
    }
  };

  const tabs = [
    { key: 'catalog' as const, label: 'Catalog', count: features.length },
    { key: 'comparison' as const, label: 'Projects comparison', count: projects.length },
  ];

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Features
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        The platform-wide feature catalog. A feature is a nav module that owns zero or more skills —
        disabling it for a project hides its navigation and turns off its skills there. Compare
        which features are enabled across every project below.
      </p>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <div className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <SegmentedTabs tabs={tabs} active={view} onChange={setView} />
        <div className="relative sm:w-64">
          <svg
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <circle cx="11" cy="11" r="7" />
            <path strokeLinecap="round" d="m21 21-4.3-4.3" />
          </svg>
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search features…"
            className="w-full rounded-xl border border-gray-200 bg-white py-2 pl-9 pr-3 text-sm text-gray-900 outline-none transition placeholder:text-gray-400 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:placeholder:text-gray-500 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30"
          />
        </div>
      </div>

      {loading ? (
        <div className="mt-4 space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonListRow key={i} />
          ))}
          <Skeleton className="h-2.5 w-24" />
        </div>
      ) : view === 'catalog' ? (
        visible.length === 0 ? (
          <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">
            {features.length === 0 ? 'No features yet.' : 'No features match your search.'}
          </p>
        ) : (
          <div className="mt-4 divide-y divide-gray-100 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:divide-gray-800 dark:border-gray-700 dark:bg-gray-800">
            {visible.map((f) => (
              <div key={f.id} className="p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                    {f.name}
                  </span>
                  <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 font-mono text-[11px] font-medium text-gray-500 dark:bg-gray-700 dark:text-gray-400">
                    {f.key}
                  </span>
                  <span className="inline-flex items-center rounded-full bg-indigo-50 px-2 py-0.5 text-[11px] font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300">
                    Enabled in {f.projects.length}/{projects.length}
                  </span>
                </div>
                <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{f.description}</p>
                {f.skill_keys.length > 0 && (
                  <div className="mt-2 flex flex-wrap items-center gap-1.5">
                    <span className="text-[11px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                      Skills
                    </span>
                    {f.skill_keys.map((k) => (
                      <span
                        key={k}
                        className="inline-flex items-center rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-[11px] text-gray-600 dark:bg-gray-700 dark:text-gray-300"
                      >
                        {k}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )
      ) : (
        <div className="mt-4">
          <ProjectMatrixTable
            itemLabel="Feature"
            rows={rows.filter((r) => matrixRowMatches(r, query))}
            projects={projects}
            busyKey={busyKey}
            onToggle={toggle}
            emptyLabel="No features match your search."
          />
        </div>
      )}
    </div>
  );
}
