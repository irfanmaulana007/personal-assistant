import { useChartTheme } from '../../../lib/useChartTheme';
import { MultiLineChart } from '../../charts/MultiLineChart';
import { StatTile, Card } from '../parts';
import { formatLatency } from '../util';
import { useGlobalDashboard, pivotByDay } from './util';

// GlobalPerformance compares request volume, errors and latency across projects
// over time. Latency percentiles are per-project figures that can't be merged
// from summaries, so the aggregate tiles show request-weighted averages instead.
export function GlobalPerformance() {
  const { perProject, aggregate } = useGlobalDashboard();
  const t = useChartTheme();
  const s = aggregate.summary;

  const requests = pivotByDay(perProject, (d) => d.requests, t.categorical);
  const errors = pivotByDay(perProject, (d) => d.errors, t.categorical);
  const latency = pivotByDay(perProject, (d) => d.avg_latency_ms, t.categorical);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile
          label="Error rate"
          value={s.requests > 0 ? `${((s.errors / s.requests) * 100).toFixed(1)}%` : '0%'}
          sub={`${s.errors.toLocaleString()} / ${s.requests.toLocaleString()}`}
        />
        <StatTile label="Requests" value={s.requests.toLocaleString()} />
        <StatTile label="Errors" value={s.errors.toLocaleString()} />
        <StatTile label="Avg latency" value={formatLatency(s.avg_latency_ms)} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Requests over time by project">
          <MultiLineChart data={requests.rows} series={requests.series} />
        </Card>
        <Card title="Errors over time by project">
          <MultiLineChart data={errors.rows} series={errors.series} />
        </Card>
      </div>

      <Card title="Average latency over time by project">
        <MultiLineChart data={latency.rows} series={latency.series} format={formatLatency} />
      </Card>
    </div>
  );
}
