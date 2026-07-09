import { Outlet } from 'react-router-dom';
import { useSearchParams } from 'react-router-dom';
import { useMetrics } from '../hooks/useMetrics';
import { DateRangePicker } from './DateRangePicker';
import { ChannelFilter } from './ChannelFilter';
import type { Channel } from '../types';

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
  const channel = (searchParams.get('channel') as Channel) || '';

  const patchParams = (patch: Record<string, string>) => {
    const sp = new URLSearchParams(searchParams);
    Object.entries(patch).forEach(([k, v]) => (v ? sp.set(k, v) : sp.delete(k)));
    setSearchParams(sp);
  };

  const { stats, loading, error } = useMetrics(from, to, channel);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900">Dashboard</h1>
          <p className="mt-0.5 text-sm text-gray-500">Your LLM usage and estimated cost.</p>
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

      {error && <p className="mt-6 text-sm text-red-600">{error}</p>}

      {loading && !stats ? (
        <p className="mt-6 text-sm text-gray-500">Loading…</p>
      ) : stats ? (
        <div className="mt-6">
          <Outlet context={{ stats }} />
        </div>
      ) : null}
    </div>
  );
}
