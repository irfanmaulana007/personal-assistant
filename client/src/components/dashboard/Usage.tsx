import { usePreferences } from '../../contexts/preferences';
import { UsageDualLineChart } from '../charts/UsageDualLineChart';
import { ModelBarChart } from '../charts/ModelBarChart';
import { Card } from './parts';
import { useDashboard, formatDayLabel } from './util';

export function Usage() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();

  return (
    <div className="space-y-6">
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
        <ModelBarChart
          formatMoney={formatMoney}
          data={stats.by_model.map((m) => ({
            model: m.model,
            tokens: m.total_tokens,
            cost: m.estimated_cost_usd,
            rateKnown: m.rate_known,
          }))}
        />
      </Card>
    </div>
  );
}
