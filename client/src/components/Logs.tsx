import { useState, useEffect } from 'react';
import { format, parseISO } from 'date-fns';
import { getLogs, getLog } from '../api/client';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { formatTokens, formatCost } from '../lib/format';
import type { Trace, Channel } from '../types';

function defaultRange(): { from: string; to: string } {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 6);
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  return { from: iso(from), to: iso(today) };
}

function fmtLatency(ms: number): string {
  if (!ms) return '—';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}

function pretty(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}

const channelBadge: Record<string, string> = {
  web: 'bg-indigo-100 text-indigo-700',
  whatsapp: 'bg-green-100 text-green-700',
};

export function Logs() {
  const [range, setRange] = useState(defaultRange);
  const [channel, setChannel] = useState<Channel>('');
  const [traces, setTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<Trace | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  useEffect(() => {
    let active = true;
    getLogs(range.from, range.to, channel)
      .then((d) => {
        if (active) {
          setTraces(d.traces ?? []);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load logs');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [range.from, range.to, channel]);

  const open = (t: Trace) => {
    setSelected(t);
    setDetailLoading(true);
    getLog(t.id)
      .then(setSelected)
      .catch(() => {})
      .finally(() => setDetailLoading(false));
  };

  return (
    <div className="flex-1 overflow-y-auto bg-gray-50 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900">Logs</h1>
          <p className="mt-0.5 text-sm text-gray-500">
            Every assistant run — inputs, tools, and outputs.
          </p>
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

      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

      <div className="mt-6 grid gap-4 lg:grid-cols-2">
        <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
          {loading ? (
            <p className="p-5 text-sm text-gray-500">Loading…</p>
          ) : traces.length === 0 ? (
            <p className="p-5 text-sm text-gray-400">No runs in this range yet.</p>
          ) : (
            <div className="max-h-[70vh] overflow-y-auto">
              {traces.map((t) => (
                <button
                  key={t.id}
                  onClick={() => open(t)}
                  className={`flex w-full items-center gap-3 border-b border-gray-50 px-4 py-3 text-left transition last:border-0 hover:bg-gray-50 ${
                    selected?.id === t.id ? 'bg-indigo-50/60' : ''
                  }`}
                >
                  <span
                    className={`h-2 w-2 shrink-0 rounded-full ${t.status === 'error' ? 'bg-red-500' : 'bg-green-500'}`}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm text-gray-800">{t.input || '(no input)'}</div>
                    <div className="mt-0.5 flex items-center gap-2 text-xs text-gray-400">
                      <span>{format(parseISO(t.created_at), 'MMM d, HH:mm')}</span>
                      <span
                        className={`rounded px-1.5 py-0.5 font-medium ${channelBadge[t.platform] ?? 'bg-gray-100 text-gray-500'}`}
                      >
                        {t.platform}
                      </span>
                      <span>{t.model || '—'}</span>
                    </div>
                  </div>
                  <div className="shrink-0 text-right text-xs text-gray-400 tabular-nums">
                    <div>{formatTokens(t.total_tokens)} tok</div>
                    <div>{fmtLatency(t.latency_ms)}</div>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm">
          {!selected ? (
            <p className="text-sm text-gray-400">Select a run to see its details.</p>
          ) : (
            <TraceDetail trace={selected} loading={detailLoading} />
          )}
        </div>
      </div>
    </div>
  );
}

function TraceDetail({ trace, loading }: { trace: Trace; loading: boolean }) {
  const meta = [
    { k: 'Model', v: trace.model || '—' },
    {
      k: 'Tokens',
      v: `${formatTokens(trace.total_tokens)} (${trace.prompt_tokens}/${trace.completion_tokens})`,
    },
    { k: 'Latency', v: fmtLatency(trace.latency_ms) },
    { k: 'Cost', v: formatCost(trace.estimated_cost_usd) },
  ];

  return (
    <div>
      <div className="flex items-center justify-between">
        <span
          className={`rounded-full px-2.5 py-1 text-xs font-medium ${
            trace.status === 'error' ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'
          }`}
        >
          {trace.status === 'error' ? 'Error' : 'OK'}
        </span>
        <span className="text-xs text-gray-400">
          {format(parseISO(trace.created_at), 'MMM d, yyyy HH:mm:ss')}
        </span>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
        {meta.map((m) => (
          <div key={m.k} className="rounded-lg bg-gray-50 px-3 py-2">
            <div className="text-[11px] uppercase tracking-wide text-gray-400">{m.k}</div>
            <div className="text-sm font-medium text-gray-800 tabular-nums">{m.v}</div>
          </div>
        ))}
      </div>

      <Section title="Input">
        <p className="whitespace-pre-wrap text-sm text-gray-800">{trace.input || '(none)'}</p>
      </Section>

      {trace.error && (
        <Section title="Error">
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-red-50 p-3 text-xs text-red-700">
            {trace.error}
          </pre>
        </Section>
      )}

      {loading ? (
        <p className="mt-4 text-xs text-gray-400">Loading tool calls…</p>
      ) : (
        trace.tools &&
        trace.tools.length > 0 && (
          <Section title={`Tool calls (${trace.tools.length})`}>
            <div className="space-y-3">
              {trace.tools.map((t, i) => (
                <div key={i} className="rounded-lg border border-gray-100">
                  <div className="border-b border-gray-100 px-3 py-2 text-sm font-medium text-indigo-700">
                    {t.name}
                  </div>
                  <div className="space-y-2 p-3">
                    <div>
                      <div className="mb-1 text-[11px] uppercase tracking-wide text-gray-400">
                        Arguments
                      </div>
                      <pre className="max-h-40 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700">
                        {pretty(t.arguments) || '{}'}
                      </pre>
                    </div>
                    <div>
                      <div className="mb-1 text-[11px] uppercase tracking-wide text-gray-400">
                        Result
                      </div>
                      <pre className="max-h-40 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700">
                        {pretty(t.result)}
                      </pre>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </Section>
        )
      )}

      <Section title="Output">
        <p className="whitespace-pre-wrap text-sm text-gray-800">{trace.output || '(none)'}</p>
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mt-5">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">{title}</h3>
      {children}
    </div>
  );
}
