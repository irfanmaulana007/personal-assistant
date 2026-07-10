import { useState, useEffect, useRef, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { getLogs, getLog } from '../api/client';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { ScoreFilter } from './ScoreFilter';
import { formatTokens } from '../lib/format';
import { usePreferences } from '../contexts/preferences';
import { Markdown } from './Markdown';
import { Skeleton } from './ui/Skeleton';
import type { Trace, TraceScore, Channel, ScoreState } from '../types';

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
  web: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300',
  whatsapp: 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300',
};

// scoreTone maps an overall 1–5 judge score to a traffic-light colour.
function scoreTone(overall: number): string {
  if (overall >= 4) return 'bg-green-100 text-green-700';
  if (overall >= 3) return 'bg-amber-100 text-amber-700';
  return 'bg-red-100 text-red-700';
}

/** A compact pill showing the judge's overall score, or an em dash if unjudged. */
function ScoreBadge({ score }: { score?: TraceScore }) {
  if (!score) return <span className="text-gray-300">—</span>;
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold tabular-nums ${scoreTone(
        score.overall,
      )}`}
      title={score.rationale}
    >
      {score.overall.toFixed(1)}
    </span>
  );
}

export function Logs() {
  const { formatDate, formatMoney } = usePreferences();
  const [searchParams, setSearchParams] = useSearchParams();
  const def = defaultRange();
  const from = searchParams.get('from') || def.from;
  const to = searchParams.get('to') || def.to;
  const channel = (searchParams.get('channel') as Channel) || '';
  const score = (searchParams.get('score') as ScoreState) || '';

  const [traces, setTraces] = useState<Trace[]>([]);
  const [loading, setLoading] = useState(true); // initial / filter-change load
  const [loadingMore, setLoadingMore] = useState(false); // appending a page
  const [error, setError] = useState('');
  const [nextCursor, setNextCursor] = useState(0);

  const [selected, setSelected] = useState<Trace | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');

  const scrollRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef(false); // guards against overlapping fetches

  // Persist filters to the URL; changing a filter resets the list.
  const patchParams = (patch: Record<string, string>) => {
    const sp = new URLSearchParams(searchParams);
    Object.entries(patch).forEach(([k, v]) => (v ? sp.set(k, v) : sp.delete(k)));
    setLoading(true);
    setSearchParams(sp);
  };

  // Load the first page and reset the accumulated list whenever filters change.
  useEffect(() => {
    let active = true;
    loadingRef.current = true;
    getLogs(from, to, channel, PAGE_SIZE, 0, score)
      .then((d) => {
        if (!active) return;
        setTraces(d.traces ?? []);
        setNextCursor(d.next_cursor ?? 0);
        setError('');
        scrollRef.current?.scrollTo({ top: 0 });
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load logs');
      })
      .finally(() => {
        if (active) {
          setLoading(false);
          loadingRef.current = false;
        }
      });
    return () => {
      active = false;
    };
  }, [from, to, channel, score]);

  // Append the next page (cursor-based) for infinite scroll.
  const loadMore = useCallback(() => {
    if (loadingRef.current || nextCursor === 0) return;
    loadingRef.current = true;
    setLoadingMore(true);
    getLogs(from, to, channel, PAGE_SIZE, nextCursor, score)
      .then((d) => {
        setTraces((prev) => [...prev, ...(d.traces ?? [])]);
        setNextCursor(d.next_cursor ?? 0);
      })
      .catch((e) => setError(e instanceof Error ? e.message : 'Failed to load more logs'))
      .finally(() => {
        setLoadingMore(false);
        loadingRef.current = false;
      });
  }, [from, to, channel, score, nextCursor]);

  // Trigger loadMore when the sentinel scrolls into view.
  useEffect(() => {
    const el = sentinelRef.current;
    const root = scrollRef.current;
    if (!el || !root) return;
    const obs = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) loadMore();
      },
      { root, rootMargin: '300px' },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [loadMore]);

  // Close the drawer on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setSelected(null);
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const openDetail = (t: Trace) => {
    setSelected(t);
    setDetailError('');
    setDetailLoading(true);
    getLog(t.id)
      .then((full) => {
        setSelected(full);
        setDetailError('');
      })
      .catch((e) =>
        // The list row (t) has no tool calls or LLM steps — those are
        // detail-only. Surface the failure instead of silently showing a
        // drawer that looks complete but is missing the tool-call section.
        setDetailError(e instanceof Error ? e.message : 'Failed to load full run detail'),
      )
      .finally(() => setDetailLoading(false));
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-gray-100 dark:bg-gray-900">
      {/* Sticky top: title + filters (outside the scroll area). */}
      <div className="shrink-0 px-6 pb-4 pt-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
              Logs
            </h1>
            <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
              Every assistant run — click a row for inputs, tools, and outputs.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <ScoreFilter value={score} onChange={(s) => patchParams({ score: s })} />
            <ChannelFilter value={channel} onChange={(c) => patchParams({ channel: c })} />
            <DateRangePicker
              from={from}
              to={to}
              onChange={(f, t) => patchParams({ from: f, to: t })}
            />
          </div>
        </div>
        {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
      </div>

      <div className="min-h-0 flex-1 px-6 pb-6">
        <div className="flex h-full flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
          <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto">
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10 bg-white dark:bg-gray-800">
                <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
                  <th className="px-4 py-2.5 font-medium">Message</th>
                  <th className="px-4 py-2.5 font-medium">User</th>
                  <th className="px-4 py-2.5 font-medium">Channel</th>
                  <th className="px-4 py-2.5 font-medium">Model</th>
                  <th className="px-4 py-2.5 text-center font-medium">Quality</th>
                  <th className="px-4 py-2.5 text-right font-medium">Tokens</th>
                  <th className="px-4 py-2.5 text-right font-medium">Duration</th>
                  <th className="px-4 py-2.5 text-right font-medium">Est. cost</th>
                  <th className="px-4 py-2.5 text-right font-medium">Time</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 8 }).map((_, i) => (
                    <tr
                      key={i}
                      className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2.5">
                          <Skeleton className="h-2 w-2 shrink-0 rounded-full" />
                          <Skeleton className="h-3.5 w-56 max-w-full" />
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="h-3.5 w-20" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="h-4 w-14 rounded" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="h-3.5 w-24" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="mx-auto h-4 w-10 rounded" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="ml-auto h-3.5 w-12" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="ml-auto h-3.5 w-12" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="ml-auto h-3.5 w-12" />
                      </td>
                      <td className="px-4 py-3">
                        <Skeleton className="ml-auto h-3.5 w-16" />
                      </td>
                    </tr>
                  ))
                ) : traces.length === 0 ? (
                  <tr>
                    <td colSpan={9} className="px-4 py-6 text-sm text-gray-400 dark:text-gray-500">
                      No runs in this range yet.
                    </td>
                  </tr>
                ) : (
                  traces.map((t) => (
                    <tr
                      key={t.id}
                      onClick={() => openDetail(t)}
                      className="cursor-pointer border-b border-gray-50 transition last:border-0 hover:bg-gray-50 dark:border-gray-800 dark:hover:bg-gray-800/60"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2.5">
                          <span
                            className={`h-2 w-2 shrink-0 rounded-full ${t.status === 'error' ? 'bg-red-500' : 'bg-green-500'}`}
                          />
                          <span className="block max-w-[22rem] truncate text-gray-800 dark:text-gray-100">
                            {t.input || '(no input)'}
                          </span>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-gray-600 dark:text-gray-300">
                        <span className="block max-w-[10rem] truncate">
                          {t.user || `#${t.user_id}`}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`rounded px-1.5 py-0.5 text-xs font-medium ${channelBadge[t.platform] ?? 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'}`}
                        >
                          {t.platform}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                        {t.model || '—'}
                      </td>
                      <td className="px-4 py-3 text-center">
                        <ScoreBadge score={t.score} />
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                        {formatTokens(t.total_tokens)}
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                        {fmtLatency(t.latency_ms)}
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                        {formatMoney(t.estimated_cost_usd)}
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-gray-400 dark:text-gray-500">
                        {formatDate(t.created_at, { time: true })}
                      </td>
                    </tr>
                  ))
                )}
                {loadingMore && (
                  <tr>
                    <td
                      colSpan={9}
                      className="px-4 py-3 text-center text-xs text-gray-400 dark:text-gray-500"
                    >
                      Loading more…
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
            {/* Infinite-scroll trigger. */}
            <div ref={sentinelRef} className="h-px" />
          </div>

          <div className="shrink-0 border-t border-gray-100 px-4 py-2.5 text-xs text-gray-400 dark:border-gray-800 dark:text-gray-500">
            {traces.length > 0
              ? `${traces.length} run${traces.length === 1 ? '' : 's'} loaded${nextCursor ? '' : ' · end'}`
              : '—'}
          </div>
        </div>

        {selected && (
          <>
            <div
              className="animate-fade-in fixed inset-0 z-40 bg-black/30 dark:bg-black/60"
              onClick={() => setSelected(null)}
              aria-hidden
            />
            <div className="animate-slide-in-right fixed right-0 top-0 z-50 flex h-full w-full max-w-2xl flex-col bg-white shadow-xl dark:bg-gray-800">
              <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700">
                <h2 className="text-base font-semibold text-gray-900 dark:text-gray-50">
                  Run detail
                </h2>
                <button
                  onClick={() => setSelected(null)}
                  aria-label="Close"
                  className="rounded-lg p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-900 dark:text-gray-500 dark:hover:bg-gray-800 dark:hover:text-gray-50"
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
              <div className="flex-1 overflow-y-auto bg-gray-50 px-6 py-5 dark:bg-gray-900">
                <TraceDetail trace={selected} loading={detailLoading} detailError={detailError} />
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function TraceDetail({
  trace,
  loading,
  detailError,
}: {
  trace: Trace;
  loading: boolean;
  detailError?: string;
}) {
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
      <div className="rounded-xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
        <div className="flex items-center justify-between">
          <span
            className={`rounded-full px-2.5 py-1 text-xs font-medium ${
              trace.status === 'error'
                ? 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
                : 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300'
            }`}
          >
            {trace.status === 'error' ? 'Error' : 'OK'}
          </span>
          <span className="text-xs text-gray-400 dark:text-gray-500">
            {formatDate(trace.created_at, { time: true, seconds: true })}
          </span>
        </div>

        <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {meta.map((m) => (
            <div key={m.k} className="rounded-lg bg-gray-50 px-3 py-2 dark:bg-gray-900">
              <div className="text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                {m.k}
              </div>
              <div className="text-sm font-medium text-gray-800 tabular-nums dark:text-gray-100">
                {m.v}
              </div>
            </div>
          ))}
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
          <span>
            <span className="text-gray-400 dark:text-gray-500">User:</span>{' '}
            {trace.user || `#${trace.user_id}`}
          </span>
          <span>
            <span className="text-gray-400 dark:text-gray-500">Channel:</span> {trace.platform}
          </span>
          {trace.skills && trace.skills.length > 0 && (
            <span className="flex flex-wrap items-center gap-1">
              <span className="text-gray-400 dark:text-gray-500">Skills:</span>
              {trace.skills.map((sk) => (
                <span
                  key={sk}
                  className="rounded bg-indigo-50 px-1.5 py-0.5 font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300"
                >
                  {sk}
                </span>
              ))}
            </span>
          )}
        </div>
      </div>

      {trace.score && (
        <Section title="Quality score">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className={`inline-flex items-center rounded-full px-2.5 py-1 text-sm font-semibold tabular-nums ${scoreTone(
                trace.score.overall,
              )}`}
            >
              {trace.score.overall.toFixed(1)} / 5
            </span>
            {(
              [
                ['Accuracy', trace.score.accuracy],
                ['Helpfulness', trace.score.helpfulness],
                ['Safety', trace.score.safety],
              ] as const
            ).map(([label, val]) => (
              <span key={label} className="rounded-lg bg-gray-50 px-2.5 py-1 text-xs text-gray-600">
                <span className="text-gray-400">{label}</span>{' '}
                <span className="font-semibold tabular-nums text-gray-800">{val}</span>
              </span>
            ))}
          </div>
          {trace.score.rationale && (
            <p className="mt-2.5 text-sm text-gray-700">{trace.score.rationale}</p>
          )}
          {trace.score.judge_model && (
            <p className="mt-1.5 text-[11px] text-gray-400">Judged by {trace.score.judge_model}</p>
          )}
        </Section>
      )}

      <Section title="Input">
        <p className="whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-100">
          {trace.input || '(none)'}
        </p>
      </Section>

      {trace.error && (
        <Section title="Error">
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-red-50 p-3 text-xs text-red-700 dark:bg-red-500/15 dark:text-red-300">
            {trace.error}
          </pre>
        </Section>
      )}

      <Section
        title={
          !loading && trace.tools && trace.tools.length > 0
            ? `Tool calls (${trace.tools.length})`
            : 'Tool calls'
        }
      >
        {loading ? (
          <div className="space-y-3">
            {Array.from({ length: 2 }).map((_, i) => (
              <div key={i} className="rounded-lg border border-gray-100 dark:border-gray-800">
                <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2 dark:border-gray-800">
                  <Skeleton className="h-3.5 w-32" />
                  <Skeleton className="h-3 w-10" />
                </div>
                <div className="px-3 py-2.5">
                  <Skeleton className="h-3 w-full" />
                </div>
              </div>
            ))}
          </div>
        ) : detailError ? (
          <p className="text-xs text-red-600 dark:text-red-400">
            Couldn't load tool calls: {detailError}
          </p>
        ) : trace.tools && trace.tools.length > 0 ? (
          <div className="space-y-3">
            {trace.tools.map((t, i) => (
              <div key={i} className="rounded-lg border border-gray-100 dark:border-gray-800">
                <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2 text-sm font-medium text-indigo-700 dark:border-gray-800 dark:text-indigo-400">
                  <span>{t.name}</span>
                  {t.latency_ms != null && (
                    <span className="text-xs font-normal text-gray-400 tabular-nums dark:text-gray-500">
                      {fmtLatency(t.latency_ms)}
                    </span>
                  )}
                </div>
                <div className="space-y-2 p-3">
                  <div>
                    <div className="mb-1 text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                      Arguments
                    </div>
                    <pre className="max-h-40 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700 dark:bg-gray-900 dark:text-gray-200">
                      {pretty(t.arguments) || '{}'}
                    </pre>
                  </div>
                  <div>
                    <div className="mb-1 text-[11px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                      Result
                    </div>
                    <pre className="max-h-40 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700 dark:bg-gray-900 dark:text-gray-200">
                      {pretty(t.result)}
                    </pre>
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-xs text-gray-400 dark:text-gray-500">
            No tools were used in this run.
          </p>
        )}
      </Section>

      {trace.steps && trace.steps.length > 0 && (
        <Section title={`LLM calls (${trace.steps.length})`}>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-gray-100 text-left uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
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
                  <tr
                    key={st.step}
                    className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                  >
                    <td className="py-1.5 pr-2 tabular-nums text-gray-500 dark:text-gray-400">
                      {st.step}
                    </td>
                    <td className="py-1.5 pr-2 text-gray-700 dark:text-gray-200">{st.model}</td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {formatTokens(st.total_tokens)} ({st.prompt_tokens}/{st.completion_tokens})
                    </td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {fmtLatency(st.latency_ms)}
                    </td>
                    <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600 dark:text-gray-300">
                      {formatMoney(st.estimated_cost_usd)}
                    </td>
                    <td className="py-1.5 text-gray-500 dark:text-gray-400">
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
          <div className="max-h-80 overflow-y-auto rounded-lg border border-gray-100 bg-gray-50/50 p-3 dark:border-gray-800 dark:bg-gray-900/50">
            <Markdown>{trace.output}</Markdown>
          </div>
        ) : (
          <p className="text-sm text-gray-400 dark:text-gray-500">(none)</p>
        )}
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
        {title}
      </h3>
      {children}
    </div>
  );
}
