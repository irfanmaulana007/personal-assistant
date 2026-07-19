import { useEffect, useState } from 'react';
import { Link, NavLink, Outlet, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { useMetrics } from '../hooks/useMetrics';
import { listProjects } from '../api/client';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { Skeleton, SkeletonCard, SkeletonStatTile } from './ui/Skeleton';
import { parseFilterList, serializeFilterList } from '../lib/filters';
import { CHANNEL_VALUES } from '../types';

function defaultRange(): { from: string; to: string } {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 29);
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  return { from: iso(from), to: iso(today) };
}

// Tabs for the scoped per-project dashboard. The unscoped /dashboard relies on
// the sidebar nav for the same set; these link relative to the project's base.
const PROJECT_TABS: { to: string; label: string; end?: boolean }[] = [
  { to: '', label: 'Overview', end: true },
  { to: 'usage', label: 'Usage' },
  { to: 'activity', label: 'Activity' },
  { to: 'performance', label: 'Performance' },
  { to: 'users', label: 'Users' },
];

function ProjectTabs({ base, search }: { base: string; search: string }) {
  return (
    <nav className="flex flex-wrap gap-1 border-b border-gray-200 dark:border-gray-700">
      {PROJECT_TABS.map((t) => (
        <NavLink
          key={t.to || 'index'}
          to={{ pathname: t.to ? `${base}/${t.to}` : base, search }}
          end={t.end}
          className={({ isActive }) =>
            `-mb-px border-b-2 px-3 py-2 text-sm font-medium transition ${
              isActive
                ? 'border-indigo-600 text-indigo-700 dark:border-indigo-400 dark:text-indigo-400'
                : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
            }`
          }
        >
          {t.label}
        </NavLink>
      ))}
    </nav>
  );
}

export function Dashboard() {
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const { id } = useParams();
  const scopedId = id ? Number(id) : undefined;

  const def = defaultRange();
  const from = searchParams.get('from') || def.from;
  const to = searchParams.get('to') || def.to;
  const channels = parseFilterList(searchParams.get('channel'), CHANNEL_VALUES);

  const patchParams = (patch: Record<string, string>) => {
    const sp = new URLSearchParams(searchParams);
    Object.entries(patch).forEach(([k, v]) => (v ? sp.set(k, v) : sp.delete(k)));
    setSearchParams(sp);
  };

  const { stats, loading, error } = useMetrics(from, to, channels, scopedId);

  // Resolve the scoped project's name. Seed instantly from router state on
  // drill-in to avoid a flash, then confirm via listProjects (superadmin sees
  // all) so a direct URL load also shows the name.
  const seededName = (location.state as { name?: string } | null)?.name;
  const [projectName, setProjectName] = useState<string | undefined>(seededName);
  useEffect(() => {
    if (!scopedId) return;
    let active = true;
    listProjects()
      .then((projects) => {
        if (!active) return;
        const p = projects.find((pr) => pr.id === scopedId);
        if (p) setProjectName(p.name);
      })
      .catch(() => {
        /* keep the seeded name / fallback label */
      });
    return () => {
      active = false;
    };
  }, [scopedId]);

  const title = scopedId ? projectName || `Project #${scopedId}` : 'Dashboard';
  const subtitle = scopedId
    ? 'Usage and estimated cost for this project.'
    : 'Your LLM and image-generation usage and estimated cost.';

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      {scopedId && (
        <Link
          to="/overview"
          className="mb-3 inline-flex items-center gap-1 text-sm font-medium text-indigo-700 hover:text-indigo-800 dark:text-indigo-400 dark:hover:text-indigo-300"
        >
          <span aria-hidden>←</span> All projects
        </Link>
      )}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            {title}
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{subtitle}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <ChannelFilter
            value={channels}
            onChange={(c) => patchParams({ channel: serializeFilterList(c) })}
          />
          <DateRangePicker
            from={from}
            to={to}
            onChange={(f, t) => patchParams({ from: f, to: t })}
          />
        </div>
      </div>

      {scopedId && (
        <div className="mt-4">
          <ProjectTabs base={`/dashboard/projects/${scopedId}`} search={location.search} />
        </div>
      )}

      {error && <p className="mt-6 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading && !stats ? (
        <div className="mt-6 space-y-6">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
            {Array.from({ length: 6 }).map((_, i) => (
              <SkeletonStatTile key={i} />
            ))}
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <SkeletonCard key={i}>
                <Skeleton className="mb-4 h-3.5 w-40" />
                <Skeleton className="h-56 w-full rounded-xl" />
              </SkeletonCard>
            ))}
          </div>
        </div>
      ) : stats ? (
        <div className="mt-6">
          <Outlet context={{ stats }} />
        </div>
      ) : null}
    </div>
  );
}
