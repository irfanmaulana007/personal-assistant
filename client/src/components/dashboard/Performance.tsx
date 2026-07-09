import { MultiLineChart } from '../charts/MultiLineChart';
import { StatTile, Card } from './parts';
import { useDashboard, formatLatency, formatDayLabel } from './util';

export function Performance() {
  const { stats } = useDashboard();
  const s = stats.summary;

  const days = stats.by_day.map((d) => ({
    label: formatDayLabel(d.date),
    requests: d.requests,
    errors: d.errors,
    latency: d.avg_latency_ms,
  }));

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile
          label="Error rate"
          value={s.requests > 0 ? `${((s.errors / s.requests) * 100).toFixed(1)}%` : '0%'}
          sub={`${s.errors.toLocaleString()} / ${s.requests.toLocaleString()}`}
        />
        <StatTile label="p50 latency" value={formatLatency(s.latency_p50_ms)} />
        <StatTile label="p95 latency" value={formatLatency(s.latency_p95_ms)} />
        <StatTile label="p99 latency" value={formatLatency(s.latency_p99_ms)} />
      </div>

      <Card title="Requests & errors over time">
        <MultiLineChart
          data={days}
          series={[
            { key: 'requests', name: 'Requests', color: '#4f46e5' },
            { key: 'errors', name: 'Errors', color: '#dc2626' },
          ]}
        />
      </Card>

      <Card title="Average latency over time">
        <MultiLineChart
          data={days}
          series={[{ key: 'latency', name: 'Avg latency', color: '#059669' }]}
          format={formatLatency}
        />
      </Card>
    </div>
  );
}
