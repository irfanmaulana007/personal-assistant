import { Outlet } from 'react-router-dom';
import { useSearchParams } from 'react-router-dom';
import { useMetrics } from '../hooks/useMetrics';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import { Skeleton, SkeletonCard, SkeletonStatTile } from './ui/Skeleton';
import { parseFilterList, serializeFilterList } from '../lib/filters';
import { CHANNEL_VALUES } from '../types';

function defaultRange(): { from: string; to: string } {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 29);
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  return { from: iso(from), to: iso(today) };
}

export function Dashboard() {
  const [searchParams, setSearchParams] = useSearchParams();
  const def = defaultRange();
  const from = searchParams.get('from') || def.from;
  const to = searchParams.get('to') || def.to;
  const channels = parseFilterList(searchParams.get('channel'), CHANNEL_VALUES);

  const patchParams = (patch: Record<string, string>) => {
    const sp = new URLSearchParams(searchParams);
    Object.entries(patch).forEach(([k, v]) => (v ? sp.set(k, v) : sp.delete(k)));
    setSearchParams(sp);
  };

  const { stats, loading, error } = useMetrics(from, to, channels);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Dashboard
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Your LLM usage and estimated cost.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
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

      {error && <p className="mt-6 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading && !stats ? (
        <div className="mt-6 space-y-6">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
            {Array.from({ length: 6 }).map((_, i) => (
              <SkeletonStatTile key={i} />
            ))}
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <SkeletonCard key={i}>
                <Skeleton className="mb-4 h-3.5 w-40" />
                <Skeleton className="h-56 w-full rounded-xl" />
              </SkeletonCard>
            ))}
          </div>
        </div>
      ) : stats ? (
        <div className="mt-6">
          <Outlet context={{ stats }} />
        </div>
      ) : null}
    </div>
  );
}
