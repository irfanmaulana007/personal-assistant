import { usePreferences } from '../../contexts/preferences';
import { ModelBarChart } from '../charts/ModelBarChart';
import { Card } from './parts';
import { useDashboard } from './util';

export function Usage() {
  const { stats } = useDashboard();
  const { formatMoney } = usePreferences();

  return (
    <div className="space-y-6">
      <Card title="Tokens by model">
        {stats.by_model.length === 0 ? (
          <p className="text-sm text-gray-400">No usage in this range yet.</p>
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
