import { formatTokens } from '../../../lib/format';
import { usePreferences } from '../../../contexts/preferences';
import { useChartTheme } from '../../../lib/useChartTheme';
import { MultiLineChart } from '../../charts/MultiLineChart';
import { HorizontalBar } from '../../charts/HorizontalBar';
import { VerticalBar } from '../../charts/VerticalBar';
import { StatTile, Card } from '../parts';
import { formatLatency, hourlyData, weekdayData, tzOffsetHours } from '../util';
import { useGlobalDashboard, pivotByDay } from './util';

// GlobalActivity compares request volume across projects over time, then shows
// the aggregate activity distributions (hours, weekday, tools, models, channels)
// summed across every selected project.
export function GlobalActivity() {
  const { perProject, aggregate } = useGlobalDashboard();
  const { prefs } = usePreferences();
  const t = useChartTheme();
  const s = aggregate.summary;
  const offset = tzOffsetHours(prefs.timezone);
  const maxReq = Math.max(...aggregate.by_platform.map((x) => x.requests), 1);

  const requests = pivotByDay(perProject, (d) => d.requests, t.categorical);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
        <StatTile label="Requests" value={s.requests.toLocaleString()} />
        <StatTile
          label="Errors"
          value={s.errors.toLocaleString()}
          sub={s.requests > 0 ? `${((s.errors / s.requests) * 100).toFixed(1)}% rate` : undefined}
        />
        <StatTile label="Tool calls" value={s.tool_calls.toLocaleString()} />
        <StatTile
          label="Avg tools / req"
          value={s.requests > 0 ? (s.tool_calls / s.requests).toFixed(2) : '0'}
        />
        <StatTile label="Avg latency" value={formatLatency(s.avg_latency_ms)} />
      </div>

      <Card title="Requests over time by project">
        <MultiLineChart data={requests.rows} series={requests.series} />
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title={`Busiest hours (${prefs.timezone === 'Asia/Jakarta' ? 'GMT+7' : 'UTC'})`}>
          <VerticalBar data={hourlyData(aggregate.by_hour, offset)} />
        </Card>
        <Card title="Requests by day of week (UTC)">
          <VerticalBar data={weekdayData(aggregate.by_weekday)} color="#059669" />
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Top tools (all projects)">
          {aggregate.top_tools.length === 0 ? (
            <p className="text-sm text-gray-400 dark:text-gray-500">No tool calls yet.</p>
          ) : (
            <HorizontalBar
              data={aggregate.top_tools.map((tool) => ({
                name: tool.tool,
                value: tool.count,
                display: `${tool.count} call${tool.count === 1 ? '' : 's'}`,
              }))}
            />
          )}
        </Card>
        <Card title="Requests by model (all projects)">
          {aggregate.by_model.length === 0 ? (
            <p className="text-sm text-gray-400 dark:text-gray-500">No usage in this range yet.</p>
          ) : (
            <HorizontalBar
              data={aggregate.by_model.map((m) => ({
                name: m.model,
                value: m.requests,
                display: `${m.requests.toLocaleString()} req`,
              }))}
            />
          )}
        </Card>
      </div>

      <Card title="By channel (all projects)">
        {aggregate.by_platform.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No data yet.</p>
        ) : (
          <div className="space-y-3">
            {aggregate.by_platform.map((p) => (
              <div key={p.platform}>
                <div className="mb-1 flex items-baseline justify-between text-sm">
                  <span className="font-medium capitalize text-gray-700 dark:text-gray-200">
                    {p.platform}
                  </span>
                  <span className="tabular-nums text-gray-500 dark:text-gray-400">
                    {p.requests.toLocaleString()} req
                    <span className="ml-2 text-gray-400 dark:text-gray-500">
                      {formatTokens(p.total_tokens)} tokens
                    </span>
                  </span>
                </div>
                <div className="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800">
                  <div
                    className="h-full rounded-full bg-indigo-500"
                    style={{ width: `${Math.max((p.requests / maxReq) * 100, 3)}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
