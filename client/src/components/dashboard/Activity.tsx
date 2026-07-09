import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { HorizontalBar } from '../charts/HorizontalBar';
import { VerticalBar } from '../charts/VerticalBar';
import { StatTile, Card } from './parts';
import { useDashboard, formatLatency, hourlyData, weekdayData, tzOffsetHours } from './util';

export function Activity() {
  const { stats } = useDashboard();
  const { prefs } = usePreferences();
  const s = stats.summary;
  const maxReq = Math.max(...stats.by_platform.map((x) => x.requests), 1);
  const offset = tzOffsetHours(prefs.timezone);

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

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title={`Busiest hours (${prefs.timezone === 'Asia/Jakarta' ? 'GMT+7' : 'UTC'})`}>
          <VerticalBar data={hourlyData(stats.by_hour, offset)} />
        </Card>
        <Card title="Requests by day of week (UTC)">
          <VerticalBar data={weekdayData(stats.by_weekday)} color="#059669" />
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Top tools">
          {stats.top_tools.length === 0 ? (
            <p className="text-sm text-gray-400">No tool calls yet.</p>
          ) : (
            <HorizontalBar
              data={stats.top_tools.map((t) => ({
                name: t.tool,
                value: t.count,
                display: `${t.count} call${t.count === 1 ? '' : 's'}`,
              }))}
            />
          )}
        </Card>
        <Card title="Requests by model">
          {stats.by_model.length === 0 ? (
            <p className="text-sm text-gray-400">No usage in this range yet.</p>
          ) : (
            <HorizontalBar
              data={stats.by_model.map((m) => ({
                name: m.model,
                value: m.requests,
                display: `${m.requests.toLocaleString()} req`,
              }))}
            />
          )}
        </Card>
      </div>

      <Card title="By platform">
        {stats.by_platform.length === 0 ? (
          <p className="text-sm text-gray-400">No data yet.</p>
        ) : (
          <div className="space-y-3">
            {stats.by_platform.map((p) => (
              <div key={p.platform}>
                <div className="mb-1 flex items-baseline justify-between text-sm">
                  <span className="font-medium capitalize text-gray-700">{p.platform}</span>
                  <span className="tabular-nums text-gray-500">
                    {p.requests.toLocaleString()} req
                    <span className="ml-2 text-gray-400">
                      {formatTokens(p.total_tokens)} tokens
                    </span>
                  </span>
                </div>
                <div className="h-2 w-full overflow-hidden rounded-full bg-gray-100">
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
