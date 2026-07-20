import { Link } from 'react-router-dom';
import { formatTokens } from '../../../lib/format';
import { usePreferences } from '../../../contexts/preferences';
import { useChartTheme } from '../../../lib/useChartTheme';
import { MultiLineChart } from '../../charts/MultiLineChart';
import { StatTile, Card } from '../parts';
import { formatLatency } from '../util';
import { useGlobalDashboard, pivotByDay } from './util';

// GlobalOverview curates the platform headline metrics plus the two core
// per-project comparison charts (tokens and requests over time) and a by-project
// summary table for drilling into a single project's dashboard.
export function GlobalOverview() {
  const { perProject, aggregate } = useGlobalDashboard();
  const { formatMoney } = usePreferences();
  const t = useChartTheme();
  const s = aggregate.summary;

  const tokens = pivotByDay(perProject, (d) => d.total_tokens, t.categorical);
  const requests = pivotByDay(perProject, (d) => d.requests, t.categorical);

  const rows = [...perProject]
    .map((pp) => ({ project: pp.project, s: pp.stats.summary }))
    .sort((a, b) => b.s.requests - a.s.requests);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        <StatTile label="Requests" value={s.requests.toLocaleString()} />
        <StatTile label="Active users" value={s.active_users.toLocaleString()} />
        <StatTile
          label="Total tokens"
          value={formatTokens(s.total_tokens)}
          sub={`${formatTokens(s.prompt_tokens)} in · ${formatTokens(s.completion_tokens)} out`}
        />
        <StatTile
          label="Est. cost"
          value={formatMoney(s.estimated_cost_usd)}
          sub={aggregate.cost_partial ? 'excludes unpriced models' : 'estimated'}
        />
        <StatTile
          label="Error rate"
          value={s.requests > 0 ? `${((s.errors / s.requests) * 100).toFixed(1)}%` : '0%'}
          sub={`${s.errors.toLocaleString()} / ${s.requests.toLocaleString()}`}
        />
        <StatTile label="Avg latency" value={formatLatency(s.avg_latency_ms)} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Tokens over time by project">
          <MultiLineChart data={tokens.rows} series={tokens.series} format={formatTokens} />
        </Card>
        <Card title="Requests over time by project">
          <MultiLineChart data={requests.rows} series={requests.series} />
        </Card>
      </div>

      <Card title="By project">
        {rows.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No projects in this range.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
                  <th className="pb-2 font-medium">Project</th>
                  <th className="pb-2 text-right font-medium">Requests</th>
                  <th className="pb-2 text-right font-medium">Tokens</th>
                  <th className="pb-2 text-right font-medium">Errors</th>
                  <th className="pb-2 text-right font-medium">Est. cost</th>
                  <th className="pb-2 font-medium" aria-label="Open dashboard" />
                </tr>
              </thead>
              <tbody>
                {rows.map(({ project, s: ps }) => (
                  <tr
                    key={project.id}
                    className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                  >
                    <td className="py-3 font-medium text-gray-800 dark:text-gray-100">
                      {project.name}
                    </td>
                    <td className="py-3 text-right tabular-nums text-gray-800 dark:text-gray-100">
                      {ps.requests.toLocaleString()}
                    </td>
                    <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {formatTokens(ps.total_tokens)}
                    </td>
                    <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {ps.errors.toLocaleString()}
                    </td>
                    <td className="py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {formatMoney(ps.estimated_cost_usd)}
                    </td>
                    <td className="py-3 pl-3 text-right">
                      <Link
                        to={`/${project.slug}/dashboard`}
                        className="inline-flex items-center gap-0.5 text-xs font-medium text-gray-400 transition hover:text-indigo-700 dark:text-gray-500 dark:hover:text-indigo-400"
                      >
                        View <span aria-hidden>→</span>
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
