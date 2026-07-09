import { useState, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { getLogs, getLog } from '../api/client';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { formatTokens } from '../lib/format';
import { usePreferences } from '../contexts/preferences';
import { Markdown } from './Markdown';
import type { Trace, Channel } from '../types';

const PAGE_SIZE = 25;

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
  const { formatDate } = usePreferences();
  const [searchParams, setSearchParams] = useSearchParams();
  const def = defaultRange();
  const from = searchParams.get('from') || def.from;
  const to = searchParams.get('to') || def.to;
  const channel = (searchParams.get('channel') as Channel) || '';

  const [traces, setTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [nextCursor, setNextCursor] = useState(0);
  const [cursor, setCursor] = useState(0);
  const [prevStack, setPrevStack] = useState<number[]>([]);

  const [selected, setSelected] = useState<Trace | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  // Persist filters to the URL; changing a filter resets pagination.
  const patchParams = (patch: Record<string, string>) => {
    const sp = new URLSearchParams(searchParams);
    Object.entries(patch).forEach(([k, v]) => (v ? sp.set(k, v) : sp.delete(k)));
    setLoading(true);
    setSearchParams(sp);
    setCursor(0);
    setPrevStack([]);
  };

  useEffect(() => {
    let active = true;
    getLogs(from, to, channel, PAGE_SIZE, cursor)
      .then((d) => {
        if (!active) return;
        setTraces(d.traces ?? []);
        setNextCursor(d.next_cursor ?? 0);
        setError('');
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
  }, [from, to, channel, cursor]);

  // Close the drawer on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setSelected(null);
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const openDetail = (t: Trace) => {
    setSelected(t);
    setDetailLoading(true);
    getLog(t.id)
      .then(setSelected)
      .catch(() => {})
      .finally(() => setDetailLoading(false));
  };

  const goNext = () => {
    if (!nextCursor) return;
    setLoading(true);
    setPrevStack((s) => [...s, cursor]);
    setCursor(nextCursor);
  };
  const goPrev = () => {
    if (prevStack.length === 0) return;
    setLoading(true);
    setPrevStack((s) => {
      const copy = [...s];
      const prev = copy.pop() ?? 0;
      setCursor(prev);
      return copy;
    });
  };

  const startIdx = prevStack.length * PAGE_SIZE;

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900">Logs</h1>
          <p className="mt-0.5 text-sm text-gray-500">
            Every assistant run — click a row for inputs, tools, and outputs.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <ChannelFilter value={channel} onChange={(c) => patchParams({ channel: c })} />
          <DateRangePicker
            from={from}
            to={to}
            onChange={(f, t) => patchParams({ from: f, to: t })}
          />
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

      <div className="mt-6 overflow-hidden rounded-2xl border border-gray-200 bg-white">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400">
                <th className="px-4 py-2.5 font-medium">Message</th>
                <th className="px-4 py-2.5 font-medium">Channel</th>
                <th className="px-4 py-2.5 font-medium">Model</th>
                <th className="px-4 py-2.5 text-right font-medium">Tokens</th>
                <th className="px-4 py-2.5 text-right font-medium">Latency</th>
                <th className="px-4 py-2.5 text-right font-medium">Time</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={6} className="px-4 py-6 text-sm text-gray-500">
                    Loading…
                  </td>
                </tr>
              ) : traces.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-6 text-sm text-gray-400">
                    No runs in this range yet.
                  </td>
                </tr>
              ) : (
                traces.map((t) => (
                  <tr
                    key={t.id}
                    onClick={() => openDetail(t)}
                    className="cursor-pointer border-b border-gray-50 transition last:border-0 hover:bg-gray-50"
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span
                          className={`h-2 w-2 shrink-0 rounded-full ${t.status === 'error' ? 'bg-red-500' : 'bg-green-500'}`}
                        />
                        <span className="block max-w-[26rem] truncate text-gray-800">
                          {t.input || '(no input)'}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`rounded px-1.5 py-0.5 text-xs font-medium ${channelBadge[t.platform] ?? 'bg-gray-100 text-gray-500'}`}
                      >
                        {t.platform}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-gray-500">{t.model || '—'}</td>
                    <td className="px-4 py-3 text-right tabular-nums text-gray-600">
                      {formatTokens(t.total_tokens)}
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-gray-600">
                      {fmtLatency(t.latency_ms)}
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-gray-400">
                      {formatDate(t.created_at, { time: true })}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        <div className="flex items-center justify-between border-t border-gray-100 px-4 py-3">
          <span className="text-xs text-gray-400">
            {traces.length > 0 ? `Showing ${startIdx + 1}–${startIdx + traces.length}` : '—'}
          </span>
          <div className="flex gap-2">
            <button
              onClick={goPrev}
              disabled={prevStack.length === 0}
              className="rounded-lg border border-gray-200 px-3 py-1.5 text-sm font-medium text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Previous
            </button>
            <button
              onClick={goNext}
              disabled={!nextCursor}
              className="rounded-lg border border-gray-200 px-3 py-1.5 text-sm font-medium text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Next
            </button>
          </div>
        </div>
      </div>

      {selected && (
        <>
          <div
            className="animate-fade-in fixed inset-0 z-40 bg-black/30"
            onClick={() => setSelected(null)}
            aria-hidden
          />
          <div className="animate-slide-in-right fixed right-0 top-0 z-50 flex h-full w-full max-w-2xl flex-col bg-white shadow-xl">
            <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4">
              <h2 className="text-base font-semibold text-gray-900">Run detail</h2>
              <button
                onClick={() => setSelected(null)}
                aria-label="Close"
                className="rounded-lg p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-900"
              >
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            </div>
            <div className="flex-1 overflow-y-auto bg-gray-50 px-6 py-5">
              <TraceDetail trace={selected} loading={detailLoading} />
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function TraceDetail({ trace, loading }: { trace: Trace; loading: boolean }) {
  const { formatDate, formatMoney } = usePreferences();
  const meta = [
    { k: 'Model', v: trace.model || '—' },
    {
      k: 'Tokens',
      v: `${formatTokens(trace.total_tokens)} (${trace.prompt_tokens}/${trace.completion_tokens})`,
    },
    { k: 'Latency', v: fmtLatency(trace.latency_ms) },
    { k: 'Cost', v: formatMoney(trace.estimated_cost_usd) },
  ];

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-gray-200 bg-white p-4">
        <div className="flex items-center justify-between">
          <span
            className={`rounded-full px-2.5 py-1 text-xs font-medium ${
              trace.status === 'error' ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'
            }`}
          >
            {trace.status === 'error' ? 'Error' : 'OK'}
          </span>
          <span className="text-xs text-gray-400">
            {formatDate(trace.created_at, { time: true, seconds: true })}
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

        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500">
          <span>
            <span className="text-gray-400">User:</span> {trace.user || `#${trace.user_id}`}
          </span>
          <span>
            <span className="text-gray-400">Channel:</span> {trace.platform}
          </span>
          {trace.skills && trace.skills.length > 0 && (
            <span className="flex flex-wrap items-center gap-1">
              <span className="text-gray-400">Skills:</span>
              {trace.skills.map((sk) => (
                <span
                  key={sk}
                  className="rounded bg-indigo-50 px-1.5 py-0.5 font-medium text-indigo-700"
                >
                  {sk}
                </span>
              ))}
            </span>
          )}
        </div>
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
                  <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2 text-sm font-medium text-indigo-700">
                    <span>{t.name}</span>
                    {t.latency_ms != null && (
                      <span className="text-xs font-normal text-gray-400 tabular-nums">
                        {fmtLatency(t.latency_ms)}
                      </span>
                    )}
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

      {trace.steps && trace.steps.length > 0 && (
        <Section title={`LLM calls (${trace.steps.length})`}>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-gray-100 text-left uppercase tracking-wide text-gray-400">
                  <th className="py-1.5 pr-2 font-medium">#</th>
                  <th className="py-1.5 pr-2 font-medium">Model</th>
                  <th className="py-1.5 pr-2 text-right font-medium">Tokens (in/out)</th>
                  <th className="py-1.5 pr-2 text-right font-medium">Latency</th>
                  <th className="py-1.5 pr-2 text-right font-medium">Cost</th>
                  <th className="py-1.5 font-medium">Finish / tools</th>
                </tr>
              </thead>
              <tbody>
                {trace.steps.map((st) => (
                  <tr key={st.step} className="border-b border-gray-50 last:border-0">
                    <td className="py-1.5 pr-2 tabular-nums text-gray-500">{st.step}</td>
                    <td className="py-1.5 pr-2 text-gray-700">{st.model}</td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600">
                      {formatTokens(st.total_tokens)} ({st.prompt_tokens}/{st.completion_tokens})
                    </td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600">
                      {fmtLatency(st.latency_ms)}
                    </td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600">
                      {formatMoney(st.estimated_cost_usd)}
                    </td>
                    <td className="py-1.5 text-gray-500">
                      {st.tool_calls && st.tool_calls.length > 0
                        ? st.tool_calls.join(', ')
                        : st.finish_reason || '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Section>
      )}

      <Section title="Output">
        {trace.output ? (
          <div className="max-h-80 overflow-y-auto rounded-lg border border-gray-100 bg-gray-50/50 p-3">
            <Markdown>{trace.output}</Markdown>
          </div>
        ) : (
          <p className="text-sm text-gray-400">(none)</p>
        )}
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-gray-200 bg-white p-4">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400">{title}</h3>
      {children}
    </div>
  );
}
