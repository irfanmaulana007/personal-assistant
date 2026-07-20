import { useEffect, useMemo, useState } from 'react';
import { Outlet, useLocation, useSearchParams } from 'react-router-dom';
import { getUsage } from '../api/client';
import { useProjects } from '../contexts/project';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { ProjectFilter } from './ProjectFilter';
import { DashboardTabs } from './dashboard/DashboardTabs';
import { Skeleton, SkeletonCard, SkeletonStatTile } from './ui/Skeleton';
import { parseFilterList, serializeFilterList } from '../lib/filters';
import { CHANNEL_VALUES } from '../types';
import { mergeStats, type PerProject } from './dashboard/global/util';

function defaultRange(): { from: string; to: string } {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 29);
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  return { from: iso(from), to: iso(today) };
}

// GlobalDashboard is the superadmin all-projects dashboard. It shares the tabbed
// body (Overview / Usage / Activity / Performance / Users) with the per-project
// dashboard, but adds a project filter and renders every time-series chart with
// one line per project so projects can be compared directly. Data is gathered by
// fanning out the per-project usage endpoint (a superadmin may read any project)
// and merging it client-side.
export function GlobalDashboard() {
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const { projects, loading: projectsLoading } = useProjects();

  const def = defaultRange();
  const from = searchParams.get('from') || def.from;
  const to = searchParams.get('to') || def.to;
  const channels = parseFilterList(searchParams.get('channel'), CHANNEL_VALUES);
  const projectsParam = searchParams.get('projects') ?? '';
  const projectIds = projectsParam.split(',').filter(Boolean);

  const patchParams = (patch: Record<string, string>) => {
    const sp = new URLSearchParams(searchParams);
    Object.entries(patch).forEach(([k, v]) => (v ? sp.set(k, v) : sp.delete(k)));
    setSearchParams(sp);
  };

  // Empty selection = every project. Keep only ids that still exist.
  const selected = useMemo(() => {
    const ids = projectsParam.split(',').filter(Boolean).map(Number);
    const wanted = new Set(ids);
    return ids.length > 0 ? projects.filter((p) => wanted.has(p.id)) : projects;
  }, [projects, projectsParam]);

  const [perProject, setPerProject] = useState<PerProject[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const channelKey = channels.join(',');
  const selectedKey = selected.map((p) => p.id).join(',');

  useEffect(() => {
    if (projectsLoading) return;
    let active = true;
    // Promise.all([]) resolves to [] immediately, so an empty selection cleanly
    // yields "no projects" without a special-cased synchronous setState here.
    Promise.all(
      selected.map((project) =>
        getUsage(from, to, channels, project.id).then((stats) => ({ project, stats })),
      ),
    )
      .then((res) => {
        if (!active) return;
        setPerProject(res);
        setError(null);
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load usage.');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [from, to, channelKey, selectedKey, projectsLoading]);

  const aggregate = useMemo(() => mergeStats(perProject ?? [], from, to), [perProject, from, to]);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Dashboard
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Platform-wide usage and estimated cost, compared across projects.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <ProjectFilter
            projects={projects}
            value={projectIds}
            onChange={(ids) => patchParams({ projects: serializeFilterList(ids) })}
          />
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

      <div className="mt-4">
        <DashboardTabs base="/dashboard" search={location.search} />
      </div>

      {error && <p className="mt-6 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {(loading || projectsLoading) && !perProject ? (
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
      ) : perProject && perProject.length === 0 ? (
        <p className="mt-6 text-sm text-gray-400 dark:text-gray-500">
          No projects match the current filter.
        </p>
      ) : perProject ? (
        <div className="mt-6">
          <Outlet context={{ perProject, aggregate }} />
        </div>
      ) : null}
    </div>
  );
}
