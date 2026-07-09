import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { UsageDualLineChart } from '../charts/UsageDualLineChart';
import { MultiLineChart } from '../charts/MultiLineChart';
import { HorizontalBar } from '../charts/HorizontalBar';
import { StatTile, Card } from './parts';
import { useDashboard, formatLatency, formatDayLabel } from './util';

// Overview curates the headline metric and one highlight chart from each of the
// other sections: Usage (tokens/cost), Performance (requests & errors),
// Activity (top tools) and Users (top users). Detail lives on the sub-pages.
export function Overview() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();
  const s = stats.summary;

  const days = stats.by_day.map((d) => ({
    label: formatDayLabel(d.date),
    tokens: d.total_tokens,
    cost: d.estimated_cost_usd,
    requests: d.requests,
    errors: d.errors,
  }));

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
          sub={stats.cost_partial ? 'excludes unpriced models' : 'estimated'}
        />
        <StatTile
          label="Error rate"
          value={s.requests > 0 ? `${((s.errors / s.requests) * 100).toFixed(1)}%` : '0%'}
          sub={`${s.errors.toLocaleString()} / ${s.requests.toLocaleString()}`}
        />
        <StatTile label="p95 latency" value={formatLatency(s.latency_p95_ms)} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Tokens & cost over time">
          <UsageDualLineChart
            formatMoney={formatMoney}
            data={days.map((d) => ({ label: d.label, tokens: d.tokens, cost: d.cost }))}
          />
        </Card>
        <Card title="Requests & errors over time">
          <MultiLineChart
            data={days.map((d) => ({ label: d.label, requests: d.requests, errors: d.errors }))}
            series={[
              { key: 'requests', name: 'Requests', color: '#4f46e5' },
              { key: 'errors', name: 'Errors', color: '#dc2626' },
            ]}
          />
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Top tools">
          {stats.top_tools.length === 0 ? (
            <p className="text-sm text-gray-400">No tool calls yet.</p>
          ) : (
            <HorizontalBar
              data={stats.top_tools.slice(0, 5).map((t) => ({
                name: t.tool,
                value: t.count,
                display: `${t.count} call${t.count === 1 ? '' : 's'}`,
              }))}
            />
          )}
        </Card>
        <Card title="Top users">
          {stats.by_user.length === 0 ? (
            <p className="text-sm text-gray-400">No usage in this range yet.</p>
          ) : (
            <HorizontalBar
              data={stats.by_user.slice(0, 5).map((u) => ({
                name: u.name?.trim() || u.email || `User #${u.user_id}`,
                value: u.requests,
                display: `${u.requests.toLocaleString()} req`,
              }))}
            />
          )}
        </Card>
      </div>
    </div>
  );
}
