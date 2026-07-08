import { useState } from 'react';
import { useMetrics } from '../hooks/useMetrics';
import { AreaChart } from './charts/AreaChart';
import { BarList } from './charts/BarList';

const RANGES = [
  { days: 7, label: '7 days' },
  { days: 30, label: '30 days' },
  { days: 90, label: '90 days' },
];

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function formatCost(n: number): string {
  if (n === 0) return '$0.00';
  if (n < 0.01) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

function formatDate(iso: string): string {
  const [, m, d] = iso.split('-');
  const months = [
    '',
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  return `${months[Number(m)]} ${Number(d)}`;
}

function StatTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm">
      <div className="text-xs font-medium uppercase tracking-wide text-gray-400">{label}</div>
      <div className="mt-1 text-2xl font-semibold text-gray-900 tabular-nums">{value}</div>
      {sub && <div className="mt-0.5 text-xs text-gray-400">{sub}</div>}
    </div>
  );
}

export function Dashboard() {
  const [days, setDays] = useState(30);
  const { stats, loading, error } = useMetrics(days);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-50 p-6">
      <div className="mx-auto max-w-4xl">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold text-gray-900">Dashboard</h1>
            <p className="mt-1 text-sm text-gray-500">Your LLM usage and estimated cost.</p>
          </div>
          <div className="flex rounded-xl border border-gray-200 bg-white p-0.5">
            {RANGES.map((r) => (
              <button
                key={r.days}
                onClick={() => setDays(r.days)}
                className={`rounded-lg px-3 py-1.5 text-sm font-medium transition ${
                  days === r.days
                    ? 'bg-indigo-100 text-indigo-700'
                    : 'text-gray-500 hover:text-gray-900'
                }`}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>

        {error && <p className="mt-6 text-sm text-red-600">{error}</p>}

        {loading && !stats ? (
          <p className="mt-6 text-sm text-gray-500">Loading…</p>
        ) : stats ? (
          <>
            <div className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
              <StatTile label="Requests" value={stats.summary.requests.toLocaleString()} />
              <StatTile
                label="Total tokens"
                value={formatTokens(stats.summary.total_tokens)}
                sub={`${formatTokens(stats.summary.prompt_tokens)} in · ${formatTokens(
                  stats.summary.completion_tokens,
                )} out`}
              />
              <StatTile
                label="Est. cost"
                value={formatCost(stats.summary.estimated_cost_usd)}
                sub={stats.cost_partial ? 'excludes unpriced models' : 'estimated'}
              />
              <StatTile
                label="Avg tokens / req"
                value={
                  stats.summary.requests > 0
                    ? formatTokens(Math.round(stats.summary.total_tokens / stats.summary.requests))
                    : '0'
                }
              />
            </div>

            <div className="mt-6 rounded-2xl border border-gray-200 bg-white p-5 shadow-sm">
              <h2 className="mb-3 text-sm font-semibold text-gray-900">Tokens per day</h2>
              <AreaChart
                data={stats.by_day.map((d) => ({
                  label: formatDate(d.date),
                  value: d.total_tokens,
                }))}
                formatValue={formatTokens}
              />
            </div>

            <div className="mt-6 rounded-2xl border border-gray-200 bg-white p-5 shadow-sm">
              <h2 className="mb-4 text-sm font-semibold text-gray-900">Tokens by model</h2>
              <BarList
                items={stats.by_model.map((m) => ({
                  label: m.model,
                  value: m.total_tokens,
                  meta: m.rate_known ? formatCost(m.estimated_cost_usd) : 'no rate',
                }))}
                formatValue={formatTokens}
              />
            </div>

            <p className="mt-4 text-xs text-gray-400">
              Cost figures are estimates based on published per-model rates and may not match your
              actual bill.
            </p>
          </>
        ) : null}
      </div>
    </div>
  );
}
