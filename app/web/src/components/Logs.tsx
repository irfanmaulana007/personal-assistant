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
import { parseFilterList, serializeFilterList } from '../lib/filters';
import { APP_VERSION_LABEL } from '../appVersion';
import { CHANNEL_VALUES, SCORE_VALUES } from '../types';
import type { Trace, TraceScore } from '../types';

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

// buildDebugText renders a full trace into a plain-text block that can be
// pasted straight into a chat/issue for debugging — every field a maintainer
// would want, including tool calls, LLM steps, the judge score, and output.
//
// Every section is emitted unconditionally, with an explicit empty marker (e.g.
// "(none)") when it has no data. That's deliberate: a debugger reading the block
// needs to tell "this run genuinely had no tool calls" apart from "the Tool
// calls section is absent" — a silently dropped section reads as missing context
// and sends debugging down the wrong path.
function buildDebugText(t: Trace): string {
  const L: string[] = [];
  const add = (label: string, value: unknown) => {
    const empty = value === undefined || value === null || value === '';
    L.push(`${label}: ${empty ? '(none)' : value}`);
  };
  // section opens a labelled block, always leaving a blank line before it.
  const section = (title: string) => {
    L.push('');
    L.push(`--- ${title} ---`);
  };

  L.push('=== RUN DETAIL ===');
  add('App version', APP_VERSION_LABEL);
  add('ID', t.id);
  add('Environment', t.environment);
  add('Status', t.status);
  add('Created', t.created_at);
  add('User', t.user ? `${t.user} (#${t.user_id})` : `#${t.user_id}`);
  add('Channel', t.platform);
  add('Source', sourceLabel(t.source) ?? t.source);
  add('Model', t.model);
  const hasImage = (t.image_total_tokens ?? 0) > 0;
  add(
    'Tokens (combined)',
    `${t.combined_total_tokens} (LLM ${t.total_tokens} + image ${t.image_total_tokens ?? 0})`,
  );
  add(
    'LLM tokens',
    `${t.total_tokens} (prompt ${t.prompt_tokens} / completion ${t.completion_tokens})`,
  );
  if (hasImage) {
    add(
      'Image tokens',
      `${t.image_total_tokens} (prompt ${t.image_prompt_tokens} / completion ${t.image_completion_tokens}) · ${t.image_model}`,
    );
  }
  add('Latency (ms)', t.latency_ms);
  add('Est. cost (USD, combined)', t.estimated_cost_usd);
  add('  LLM cost (USD)', t.llm_cost_usd);
  if (hasImage) add('  Image cost (USD)', t.image_cost_usd);
  add('Skills', t.skills && t.skills.length > 0 ? t.skills.join(', ') : '');

  section('Quality score');
  if (t.score) {
    add('Overall', `${t.score.overall} / 5`);
    add('Accuracy', t.score.accuracy);
    add('Helpfulness', t.score.helpfulness);
    add('Safety', t.score.safety);
    add('Judge model', t.score.judge_model);
    add('Rationale', t.score.rationale);
  } else if (t.status === 'error') {
    L.push('(failed — no reply to score)');
  } else {
    L.push('(not scored)');
  }

  section('Input');
  L.push(t.input || '(none)');

  section('Error');
  L.push(t.error || '(none)');

  section(`Tool calls (${t.tools?.length ?? 0})`);
  if (t.tools && t.tools.length > 0) {
    t.tools.forEach((tool, i) => {
      L.push(`[${i + 1}] ${tool.name}${tool.latency_ms != null ? ` (${tool.latency_ms}ms)` : ''}`);
      if (tool.total_tokens) {
        L.push(
          `  usage: ${tool.model} · ${tool.total_tokens} tok (${tool.prompt_tokens ?? 0}/${tool.completion_tokens ?? 0}) · $${tool.estimated_cost_usd ?? 0}`,
        );
      }
      L.push(`  arguments: ${pretty(tool.arguments) || '{}'}`);
      L.push(`  result: ${pretty(tool.result)}`);
    });
  } else {
    L.push('(none)');
  }

  section(`LLM calls (${t.steps?.length ?? 0})`);
  if (t.steps && t.steps.length > 0) {
    t.steps.forEach((st) => {
      const finish =
        st.tool_calls && st.tool_calls.length > 0
          ? st.tool_calls.join(', ')
          : st.finish_reason || '—';
      L.push(
        `#${st.step} ${st.model} · ${st.total_tokens} tok (${st.prompt_tokens}/${st.completion_tokens}) · ${st.latency_ms}ms · $${st.estimated_cost_usd} · ${finish}`,
      );
    });
  } else {
    L.push('(none)');
  }

  section('Output');
  L.push(t.output || '(none)');

  return L.join('\n');
}

const channelBadge: Record<string, string> = {
  web: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300',
  whatsapp: 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300',
};

// Human labels for non-interactive trace sources (scheduled routines). An
// interactive run has source "chat" (or empty) and gets no badge — only runs
// triggered by a routine are worth marking apart from ordinary chats.
const SOURCE_LABELS: Record<string, string> = {
  start_of_day: 'Start of day',
  end_of_day: 'End of day',
};

// sourceLabel returns a display label for a routine-triggered run, or null for
// an ordinary interactive chat (which needs no badge).
function sourceLabel(source: string | undefined): string | null {
  if (!source || source === 'chat') return null;
  return SOURCE_LABELS[source] ?? source;
}

const sourceBadgeClass =
  'rounded px-1.5 py-0.5 text-xs font-medium bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300';

// scoreTone maps an overall 1–5 judge score to a traffic-light colour.
function scoreTone(overall: number): string {
  if (overall >= 4) return 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300';
  if (overall >= 3) return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300';
  return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300';
}

/** A compact pill showing the judge's overall score, a "Failed" pill for error
 *  runs (which never produced a reply to judge), or an em dash if simply
 *  unjudged. "Failed" is text, not a 1–5 number, so it reads distinctly from a
 *  genuinely low score. */
function ScoreBadge({ score, status }: { score?: TraceScore; status?: string }) {
  if (score)
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
  if (status === 'error')
    return (
      <span
        className="inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 text-xs font-medium text-red-600 ring-1 ring-inset ring-red-200 dark:bg-red-500/10 dark:text-red-300 dark:ring-red-500/30"
        title="This run failed before producing a reply, so it can't be scored."
      >
        Failed
      </span>
    );
  return <span className="text-gray-300 dark:text-gray-600">—</span>;
}

export function Logs() {
  const { formatDate, formatMoney } = usePreferences();
  const [searchParams, setSearchParams] = useSearchParams();
  const def = defaultRange();
  const from = searchParams.get('from') || def.from;
  const to = searchParams.get('to') || def.to;
  const channels = parseFilterList(searchParams.get('channel'), CHANNEL_VALUES);
  const scores = parseFilterList(searchParams.get('score'), SCORE_VALUES);
  // Stable string keys so the fetch effects re-run only when a selection
  // actually changes (arrays are new objects on every render).
  const channelKey = channels.join(',');
  const scoreKey = scores.join(',');

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
    getLogs(from, to, channels, PAGE_SIZE, 0, scores)
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [from, to, channelKey, scoreKey]);

  // Append the next page (cursor-based) for infinite scroll.
  const loadMore = useCallback(() => {
    if (loadingRef.current || nextCursor === 0) return;
    loadingRef.current = true;
    setLoadingMore(true);
    getLogs(from, to, channels, PAGE_SIZE, nextCursor, scores)
      .then((d) => {
        setTraces((prev) => [...prev, ...(d.traces ?? [])]);
        setNextCursor(d.next_cursor ?? 0);
      })
      .catch((e) => setError(e instanceof Error ? e.message : 'Failed to load more logs'))
      .finally(() => {
        setLoadingMore(false);
        loadingRef.current = false;
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [from, to, channelKey, scoreKey, nextCursor]);

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
            <ScoreFilter
              value={scores}
              onChange={(s) => patchParams({ score: serializeFilterList(s) })}
            />
            <ChannelFilter
              value={channels}
              onChange={(c) => patchParams({ channel: serializeFilterList(c) })}
            />
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
                        <div className="flex flex-wrap items-center gap-1">
                          <span
                            className={`rounded px-1.5 py-0.5 text-xs font-medium ${channelBadge[t.platform] ?? 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'}`}
                          >
                            {t.platform}
                          </span>
                          {sourceLabel(t.source) && (
                            <span className={sourceBadgeClass}>{sourceLabel(t.source)}</span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                        {t.model || '—'}
                      </td>
                      <td className="px-4 py-3 text-center">
                        <ScoreBadge score={t.score} status={t.status} />
                      </td>
                      <td className="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-gray-300">
                        {formatTokens(t.combined_total_tokens ?? t.total_tokens)}
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
                <div className="flex items-center gap-1">
                  {/* Gate the copy until the full trace has loaded: the drawer
                      first shows the lightweight list-row trace, which carries
                      no tool calls, LLM calls, or skills. Copying then would
                      silently produce an incomplete debug dump. */}
                  <CopyButton
                    getText={() => buildDebugText(selected)}
                    disabled={detailLoading || !!detailError}
                    disabledTitle={
                      detailError
                        ? 'Run detail failed to load — reopen the run to copy full context'
                        : 'Loading full run detail…'
                    }
                  />
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
  const hasImage = (trace.image_total_tokens ?? 0) > 0;
  const combinedTokens = trace.combined_total_tokens ?? trace.total_tokens;
  const meta = [
    { k: 'Model', v: trace.model || '—' },
    {
      k: hasImage ? 'Tokens (combined)' : 'Tokens',
      v: hasImage
        ? formatTokens(combinedTokens)
        : `${formatTokens(trace.total_tokens)} (${trace.prompt_tokens}/${trace.completion_tokens})`,
    },
    { k: 'Latency', v: fmtLatency(trace.latency_ms) },
    { k: hasImage ? 'Cost (combined)' : 'Cost', v: formatMoney(trace.estimated_cost_usd) },
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
          {sourceLabel(trace.source) && (
            <span className="flex items-center gap-1">
              <span className="text-gray-400 dark:text-gray-500">Source:</span>
              <span className={sourceBadgeClass}>{sourceLabel(trace.source)}</span>
            </span>
          )}
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

      {hasImage && (
        <Section title="Tokens & cost breakdown">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-gray-100 text-left uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
                  <th className="py-1.5 pr-2 font-medium">Source</th>
                  <th className="py-1.5 pr-2 font-medium">Model</th>
                  <th className="py-1.5 pr-2 text-right font-medium">Tokens (in/out)</th>
                  <th className="py-1.5 text-right font-medium">Cost</th>
                </tr>
              </thead>
              <tbody>
                <tr className="border-b border-gray-50 dark:border-gray-800">
                  <td className="py-1.5 pr-2 text-gray-700 dark:text-gray-200">LLM</td>
                  <td className="py-1.5 pr-2 text-gray-500 dark:text-gray-400">
                    {trace.model || '—'}
                  </td>
                  <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600 dark:text-gray-300">
                    {formatTokens(trace.total_tokens)} ({trace.prompt_tokens}/
                    {trace.completion_tokens})
                  </td>
                  <td className="py-1.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                    {formatMoney(trace.llm_cost_usd)}
                  </td>
                </tr>
                <tr className="border-b border-gray-50 dark:border-gray-800">
                  <td className="py-1.5 pr-2 text-gray-700 dark:text-gray-200">Image generation</td>
                  <td className="py-1.5 pr-2 text-gray-500 dark:text-gray-400">
                    {trace.image_model || '—'}
                  </td>
                  <td className="py-1.5 pr-2 text-right tabular-nums text-gray-600 dark:text-gray-300">
                    {formatTokens(trace.image_total_tokens ?? 0)} ({trace.image_prompt_tokens ?? 0}/
                    {trace.image_completion_tokens ?? 0})
                  </td>
                  <td className="py-1.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                    {formatMoney(trace.image_cost_usd)}
                  </td>
                </tr>
                <tr className="font-semibold text-gray-800 dark:text-gray-100">
                  <td className="py-1.5 pr-2">Combined</td>
                  <td className="py-1.5 pr-2" />
                  <td className="py-1.5 pr-2 text-right tabular-nums">
                    {formatTokens(combinedTokens)}
                  </td>
                  <td className="py-1.5 text-right tabular-nums">
                    {formatMoney(trace.estimated_cost_usd)}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </Section>
      )}

      <Section title="Quality score">
        {trace.score ? (
          <>
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
                <span
                  key={label}
                  className="rounded-lg bg-gray-50 px-2.5 py-1 text-xs text-gray-600 dark:bg-gray-700/50 dark:text-gray-300"
                >
                  <span className="text-gray-400 dark:text-gray-500">{label}</span>{' '}
                  <span className="font-semibold tabular-nums text-gray-800 dark:text-gray-100">
                    {val}
                  </span>
                </span>
              ))}
            </div>
            {trace.score.rationale && (
              <p className="mt-2.5 text-sm text-gray-700 dark:text-gray-300">
                {trace.score.rationale}
              </p>
            )}
            {trace.score.judge_model && (
              <p className="mt-1.5 text-[11px] text-gray-400 dark:text-gray-500">
                Judged by {trace.score.judge_model}
              </p>
            )}
          </>
        ) : trace.status === 'error' ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="inline-flex items-center rounded-full bg-red-50 px-2.5 py-1 text-sm font-medium text-red-600 ring-1 ring-inset ring-red-200 dark:bg-red-500/10 dark:text-red-300 dark:ring-red-500/30">
              Failed
            </span>
            <span className="text-sm text-gray-500 dark:text-gray-400">
              This run failed before producing a reply, so it can't be scored.
            </span>
          </div>
        ) : (
          <EmptyState>This run has not been scored.</EmptyState>
        )}
      </Section>

      <Section title="Input">
        {trace.input ? (
          <p className="whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-100">
            {trace.input}
          </p>
        ) : (
          <EmptyState>No input was recorded for this run.</EmptyState>
        )}
      </Section>

      <Section title="Error">
        {trace.error ? (
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-red-50 p-3 text-xs text-red-700 dark:bg-red-500/15 dark:text-red-300">
            {trace.error}
          </pre>
        ) : (
          <EmptyState>No errors in this run.</EmptyState>
        )}
      </Section>

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
                  <span className="flex items-center gap-2 text-xs font-normal text-gray-400 tabular-nums dark:text-gray-500">
                    {t.total_tokens ? (
                      <span title={t.model}>
                        {formatTokens(t.total_tokens)} tok ·{' '}
                        {formatMoney(t.estimated_cost_usd ?? 0)}
                      </span>
                    ) : null}
                    {t.latency_ms != null && <span>{fmtLatency(t.latency_ms)}</span>}
                  </span>
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
          <EmptyState>No tools were used in this run.</EmptyState>
        )}
      </Section>

      <Section
        title={
          trace.steps && trace.steps.length > 0 ? `LLM calls (${trace.steps.length})` : 'LLM calls'
        }
      >
        {trace.steps && trace.steps.length > 0 ? (
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
        ) : (
          <EmptyState>No LLM calls were recorded for this run.</EmptyState>
        )}
      </Section>

      <Section title="Output">
        {trace.output ? (
          <div className="max-h-80 overflow-y-auto rounded-lg border border-gray-100 bg-gray-50/50 p-3 dark:border-gray-800 dark:bg-gray-900/50">
            <Markdown>{trace.output}</Markdown>
          </div>
        ) : (
          <EmptyState>No output was produced for this run.</EmptyState>
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

function EmptyState({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-gray-400 dark:text-gray-500">{children}</p>;
}

/**
 * CopyButton copies the text produced by getText() to the clipboard, showing a
 * brief check-mark confirmation. Used in the run-detail header to grab the whole
 * trace as a paste-ready debug dump.
 *
 * `disabled` gates copying until the source data is complete — the run-detail
 * drawer opens on a lightweight list-row trace (no tool calls / LLM calls /
 * skills) and only swaps in the full trace once the detail request resolves.
 * Copying before then would yield a debug dump missing exactly those sections.
 */
function CopyButton({
  getText,
  disabled = false,
  disabledTitle,
}: {
  getText: () => string;
  disabled?: boolean;
  disabledTitle?: string;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    if (disabled) return;
    const text = getText();
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Fallback for insecure contexts / browsers without the async clipboard API.
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try {
        document.execCommand('copy');
      } catch {
        /* give up silently — nothing more we can do */
      }
      document.body.removeChild(ta);
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <button
      onClick={copy}
      disabled={disabled}
      aria-label={copied ? 'Copied' : 'Copy debug details'}
      title={
        disabled
          ? (disabledTitle ?? 'Copy debug details')
          : copied
            ? 'Copied!'
            : 'Copy debug details'
      }
      className={`rounded-lg p-1.5 transition ${
        disabled
          ? 'cursor-not-allowed text-gray-300 dark:text-gray-600'
          : copied
            ? 'text-green-600 dark:text-green-400'
            : 'text-gray-400 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-500 dark:hover:bg-gray-800 dark:hover:text-gray-50'
      }`}
    >
      {copied ? (
        <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
        </svg>
      ) : (
        <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <rect x="9" y="9" width="11" height="11" rx="2" strokeWidth={2} />
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M5 15V5a2 2 0 012-2h10"
          />
        </svg>
      )}
    </button>
  );
}
