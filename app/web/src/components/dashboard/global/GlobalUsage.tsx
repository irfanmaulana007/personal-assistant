import { formatTokens } from '../../../lib/format';
import { usePreferences } from '../../../contexts/preferences';
import { useChartTheme } from '../../../lib/useChartTheme';
import { MultiLineChart } from '../../charts/MultiLineChart';
import { ModelBarChart } from '../../charts/ModelBarChart';
import { StatTile, Card } from '../parts';
import { useGlobalDashboard, pivotByDay } from './util';

// GlobalUsage compares token and cost consumption across projects over time and
// shows the aggregate token spend per model.
export function GlobalUsage() {
  const { perProject, aggregate } = useGlobalDashboard();
  const { formatMoney } = usePreferences();
  const t = useChartTheme();
  const s = aggregate.summary;

  const tokens = pivotByDay(perProject, (d) => d.total_tokens, t.categorical);
  const cost = pivotByDay(perProject, (d) => d.estimated_cost_usd, t.categorical);

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
          sub={aggregate.cost_partial ? 'excludes unpriced models' : 'estimated'}
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

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Tokens over time by project">
          <MultiLineChart data={tokens.rows} series={tokens.series} format={formatTokens} />
        </Card>
        <Card title="Cost over time by project">
          <MultiLineChart data={cost.rows} series={cost.series} format={formatMoney} />
        </Card>
      </div>

      <Card title="Tokens by model (all projects)">
        {aggregate.by_model.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500">No usage in this range yet.</p>
        ) : (
          <ModelBarChart
            formatMoney={formatMoney}
            data={aggregate.by_model.map((m) => ({
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
