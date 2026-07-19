import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';
import { getAdminOverview } from '../../api/client';
import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { useChartTheme } from '../../lib/useChartTheme';
import { StatTile } from './parts';
import { SkeletonStatTile, SkeletonCard, Skeleton } from '../ui/Skeleton';
import type { AdminOverview, ProjectOverviewRow } from '../../types';

// ProjectsOverview is the superadmin cross-project usage view: summary stat
// tiles, a per-project breakdown table, and a requests-per-project bar chart.
// A project filter narrows every part to a single project. The default range
// (last 30 days) is decided by the server when no from/to is passed.
export function ProjectsOverview() {
  const { formatMoney } = usePreferences();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // 0 = "All projects"; otherwise a specific project_id.
  const [projectId, setProjectId] = useState(0);

  // Keep the full project list stable across filtered fetches so the dropdown
  // never collapses to a single option once a project is selected.
  const [allProjects, setAllProjects] = useState<ProjectOverviewRow[]>([]);

  useEffect(() => {
    let active = true;
    getAdminOverview(undefined, undefined, projectId || undefined)
      .then((res) => {
        if (!active) return;
        setData(res);
        setError(null);
        // Only the unfiltered response carries every project; seed the filter
        // options from it and leave them untouched while a filter is applied.
        if (projectId === 0) setAllProjects(res.projects);
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load overview.');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [projectId]);

  const rows = useMemo(
    () => (data ? [...data.projects].sort((a, b) => b.requests - a.requests) : []),
    [data],
  );

  const filterOptions = allProjects.length > 0 ? allProjects : (data?.projects ?? []);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Dashboard
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Platform-wide usage and cost across all projects.
          </p>
        </div>
        <label className="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
          <span>Project</span>
          <select
            value={projectId}
            onChange={(e) => setProjectId(Number(e.target.value))}
            className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-sm text-gray-900 outline-none focus:border-indigo-500 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400"
          >
            <option value={0}>All projects</option>
            {filterOptions.map((p) => (
              <option key={p.project_id} value={p.project_id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="mt-6 space-y-6">
        {error ? (
          <div className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
            {error}
          </div>
        ) : loading || !data ? (
          <LoadingState />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
              <StatTile label="Total requests" value={data.summary.requests.toLocaleString()} />
              <StatTile label="Total tokens" value={formatTokens(data.summary.total_tokens)} />
              <StatTile
                label="Est. cost"
                value={formatMoney(data.summary.estimated_cost_usd)}
                sub="estimated"
              />
              <StatTile label="Errors" value={data.summary.errors.toLocaleString()} />
              <StatTile label="Active users" value={data.summary.active_users.toLocaleString()} />
            </div>

            <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
              <h3 className="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-50">
                By project
              </h3>
              {rows.length === 0 ? (
                <p className="text-sm text-gray-400 dark:text-gray-500">
                  No projects in this range.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
                        <th className="pb-2 font-medium">Project</th>
                        <th className="pb-2 text-right font-medium">Members</th>
                        <th className="pb-2 text-right font-medium">Enabled skills</th>
                        <th className="pb-2 text-right font-medium">Requests</th>
                        <th className="pb-2 text-right font-medium">Tokens</th>
                        <th className="pb-2 text-right font-medium">Est. cost</th>
                        <th className="pb-2 font-medium" aria-label="Open dashboard" />
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((p) => {
                        const open = () =>
                          navigate(`/dashboard/projects/${p.project_id}`, {
                            state: { name: p.name },
                          });
                        return (
                          <tr
                            key={p.project_id}
                            role="button"
                            tabIndex={0}
                            onClick={open}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' || e.key === ' ') {
                                e.preventDefault();
                                open();
                              }
                            }}
                            title={`Open ${p.name} dashboard`}
                            className="group cursor-pointer border-b border-gray-50 last:border-0 hover:bg-gray-50 focus:bg-gray-50 focus:outline-none dark:border-gray-800 dark:hover:bg-gray-700/40 dark:focus:bg-gray-700/40"
                          >
                            <td className="py-3 font-medium text-gray-800 dark:text-gray-100">
                              {p.name}
                            </td>
                            <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                              {p.member_count.toLocaleString()}
                            </td>
                            <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                              {p.enabled_skills.toLocaleString()}
                            </td>
                            <td className="py-3 text-right tabular-nums text-gray-800 dark:text-gray-100">
                              {p.requests.toLocaleString()}
                            </td>
                            <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                              {formatTokens(p.total_tokens)}
                            </td>
                            <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                              {formatMoney(p.estimated_cost_usd)}
                            </td>
                            <td className="py-3 pl-3 text-right">
                              <span className="inline-flex items-center gap-0.5 text-xs font-medium text-gray-400 transition group-hover:text-indigo-700 dark:text-gray-500 dark:group-hover:text-indigo-400">
                                View <span aria-hidden>→</span>
                              </span>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
              <h3 className="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-50">
                Requests by project
              </h3>
              <RequestsByProjectChart rows={rows} />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

interface ChartDatum {
  name: string;
  requests: number;
}

interface TooltipEntry {
  payload: ChartDatum;
}

function ChartTooltip({ active, payload }: { active?: boolean; payload?: TooltipEntry[] }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm dark:border-gray-700 dark:bg-gray-800">
      <div className="font-medium text-gray-900 dark:text-gray-100">{p.name}</div>
      <div className="text-gray-500 dark:text-gray-400">
        {p.requests.toLocaleString()} request{p.requests === 1 ? '' : 's'}
      </div>
    </div>
  );
}

// RequestsByProjectChart plots requests per project for up to the top 10 rows.
// Colors come entirely from useChartTheme so the SVG tracks the active theme.
function RequestsByProjectChart({ rows }: { rows: ProjectOverviewRow[] }) {
  const t = useChartTheme();
  const data: ChartDatum[] = rows.slice(0, 10).map((p) => ({ name: p.name, requests: p.requests }));

  if (data.length === 0) {
    return <p className="text-sm text-gray-400 dark:text-gray-500">No data yet.</p>;
  }

  return (
    <div className="overflow-x-auto">
      <div style={{ minWidth: Math.max(320, data.length * 72) }}>
        <ResponsiveContainer width="100%" height={280}>
          <BarChart data={data} margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
            <CartesianGrid vertical={false} stroke={t.grid} />
            <XAxis
              dataKey="name"
              tick={{ fontSize: 11, fill: t.axis }}
              axisLine={false}
              tickLine={false}
              interval={0}
            />
            <YAxis
              tickFormatter={(v: number) => v.toLocaleString()}
              tick={{ fontSize: 11, fill: t.axis }}
              axisLine={false}
              tickLine={false}
              width={48}
            />
            <Tooltip content={<ChartTooltip />} cursor={{ fill: t.cursorFill }} />
            <Bar
              dataKey="requests"
              fill={t.indigo}
              radius={[4, 4, 0, 0]}
              maxBarSize={48}
              isAnimationActive={false}
            />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function LoadingState() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <SkeletonStatTile key={i} />
        ))}
      </div>
      <SkeletonCard>
        <Skeleton className="h-4 w-28" />
        <div className="mt-4 space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-8 w-full" />
          ))}
        </div>
      </SkeletonCard>
      <SkeletonCard>
        <Skeleton className="h-4 w-36" />
        <Skeleton className="mt-4 h-64 w-full" />
      </SkeletonCard>
    </div>
  );
}
