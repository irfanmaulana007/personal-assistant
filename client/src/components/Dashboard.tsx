import { useState } from 'react';
import { useMetrics } from '../hooks/useMetrics';
import { UsageAreaChart } from './charts/UsageAreaChart';
import { ModelBarChart } from './charts/ModelBarChart';
import { HorizontalBar } from './charts/HorizontalBar';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { formatTokens, formatCost } from '../lib/format';
import type { Channel } from '../types';

function defaultRange(): { from: string; to: string } {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 29);
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  return { from: iso(from), to: iso(today) };
}

function formatDayLabel(iso: string): string {
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

function formatLatency(ms: number): string {
  if (!ms) return '—';
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${ms}ms`;
}

function StatTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-4">
      <div className="text-[11px] font-medium uppercase tracking-wide text-gray-400">{label}</div>
      <div className="mt-1.5 text-2xl font-semibold tracking-tight text-gray-900 tabular-nums">
        {value}
      </div>
      {sub && <div className="mt-1 text-xs text-gray-400">{sub}</div>}
    </div>
  );
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-5">
      <h2 className="mb-4 text-sm font-semibold text-gray-900">{title}</h2>
      {children}
    </div>
  );
}

export function Dashboard() {
  const [range, setRange] = useState(defaultRange);
  const [channel, setChannel] = useState<Channel>('');
  const { stats, loading, error } = useMetrics(range.from, range.to, channel);

  const isEmpty = stats && stats.summary.requests === 0;

  return (
    <div className="flex-1 overflow-y-auto bg-gray-50 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900">Dashboard</h1>
          <p className="mt-0.5 text-sm text-gray-500">Your LLM usage and estimated cost.</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <ChannelFilter value={channel} onChange={setChannel} />
          <DateRangePicker
            from={range.from}
            to={range.to}
            onChange={(from, to) => setRange({ from, to })}
          />
        </div>
      </div>

      {error && <p className="mt-6 text-sm text-red-600">{error}</p>}

      {loading && !stats ? (
        <p className="mt-6 text-sm text-gray-500">Loading…</p>
      ) : stats ? (
        <>
          <div className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-7">
            <StatTile label="Requests" value={stats.summary.requests.toLocaleString()} />
            <StatTile
              label="Errors"
              value={stats.summary.errors.toLocaleString()}
              sub={
                stats.summary.requests > 0
                  ? `${((stats.summary.errors / stats.summary.requests) * 100).toFixed(1)}% rate`
                  : undefined
              }
            />
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
            <StatTile label="Tool calls" value={stats.summary.tool_calls.toLocaleString()} />
            <StatTile label="Avg latency" value={formatLatency(stats.summary.avg_latency_ms)} />
            <StatTile
              label="Avg tokens / req"
              value={
                stats.summary.requests > 0
                  ? formatTokens(Math.round(stats.summary.total_tokens / stats.summary.requests))
                  : '0'
              }
            />
          </div>

          {isEmpty ? (
            <div className="mt-6 rounded-2xl border border-dashed border-gray-200 bg-white p-10 text-center">
              <p className="text-sm font-medium text-gray-600">No usage in this range yet</p>
              <p className="mt-1 text-sm text-gray-400">
                Chat with the assistant to start tracking tokens, cost, and tools here.
              </p>
            </div>
          ) : (
            <>
              <div className="mt-6">
                <Card title="Tokens per day">
                  <UsageAreaChart
                    data={stats.by_day.map((d) => ({
                      label: formatDayLabel(d.date),
                      value: d.total_tokens,
                    }))}
                  />
                </Card>
              </div>

              <div className="mt-6 grid gap-4 lg:grid-cols-2">
                <Card title="Tokens by model">
                  <ModelBarChart
                    data={stats.by_model.map((m) => ({
                      model: m.model,
                      tokens: m.total_tokens,
                      cost: m.estimated_cost_usd,
                      rateKnown: m.rate_known,
                    }))}
                  />
                </Card>
                <Card title="Top tools">
                  <HorizontalBar
                    data={stats.top_tools.map((t) => ({
                      name: t.tool,
                      value: t.count,
                      display: `${t.count} call${t.count === 1 ? '' : 's'}`,
                    }))}
                  />
                </Card>
              </div>

              <div className="mt-6">
                <Card title="By platform">
                  {stats.by_platform.length === 0 ? (
                    <p className="text-sm text-gray-400">No data yet.</p>
                  ) : (
                    <div className="space-y-3">
                      {stats.by_platform.map((p) => {
                        const maxReq = Math.max(...stats.by_platform.map((x) => x.requests), 1);
                        return (
                          <div key={p.platform}>
                            <div className="mb-1 flex items-baseline justify-between text-sm">
                              <span className="font-medium capitalize text-gray-700">
                                {p.platform}
                              </span>
                              <span className="text-gray-500 tabular-nums">
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
                        );
                      })}
                    </div>
                  )}
                </Card>
              </div>

              <p className="mt-4 text-xs text-gray-400">
                Cost figures are estimates based on published per-model rates and may not match your
                actual bill.
              </p>
            </>
          )}
        </>
      ) : null}
    </div>
  );
}
