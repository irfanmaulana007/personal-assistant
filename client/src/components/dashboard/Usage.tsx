import { formatTokens } from '../../lib/format';
import { usePreferences } from '../../contexts/preferences';
import { UsageDualLineChart } from '../charts/UsageDualLineChart';
import { ModelBarChart } from '../charts/ModelBarChart';
import { StatTile, Card } from './parts';
import { useDashboard, formatDayLabel } from './util';

export function Usage() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();
  const s = stats.summary;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile
          label="Total tokens"
          value={formatTokens(s.total_tokens)}
          sub={`${formatTokens(s.prompt_tokens)} in · ${formatTokens(s.completion_tokens)} out`}
        />
        <StatTile label="Prompt tokens" value={formatTokens(s.prompt_tokens)} />
        <StatTile label="Completion tokens" value={formatTokens(s.completion_tokens)} />
        <StatTile
          label="Est. cost"
          value={formatMoney(s.estimated_cost_usd)}
          sub={stats.cost_partial ? 'excludes unpriced models' : 'estimated'}
        />
        <StatTile
          label="Avg tokens / req"
          value={s.requests > 0 ? formatTokens(Math.round(s.total_tokens / s.requests)) : '0'}
        />
        <StatTile
          label="Cost / req"
          value={s.requests > 0 ? formatMoney(s.estimated_cost_usd / s.requests) : formatMoney(0)}
        />
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

      <Card title="Tokens by model">
        {stats.by_model.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No usage in this range yet.</p>
        ) : (
          <ModelBarChart
            formatMoney={formatMoney}
            data={stats.by_model.map((m) => ({
              model: m.model,
              tokens: m.total_tokens,
              cost: m.estimated_cost_usd,
              rateKnown: m.rate_known,
            }))}
          />
        )}
      </Card>
    </div>
  );
}
