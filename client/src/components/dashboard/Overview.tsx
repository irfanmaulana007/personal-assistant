import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { UsageDualLineChart } from '../charts/UsageDualLineChart';
import { StatTile, Card } from './parts';
import { useDashboard, formatLatency, formatDayLabel } from './util';

// Overview curates the single headline metric from each of the other sections:
// volume (Activity) + users (Users) + tokens/cost (Usage) + reliability &
// latency (Performance), plus the tokens/cost trend. Detail lives on the sub-pages.
export function Overview() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();
  const s = stats.summary;

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

      <Card title="Tokens & cost over time">
        <UsageDualLineChart
          formatMoney={formatMoney}
          data={stats.by_day.map((d) => ({
            label: formatDayLabel(d.date),
            tokens: d.total_tokens,
            cost: d.estimated_cost_usd,
          }))}
        />
      </Card>
    </div>
  );
}
