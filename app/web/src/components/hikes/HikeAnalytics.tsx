import { useMemo, type ReactNode } from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  PieChart,
  Pie,
  Cell,
} from 'recharts';
import type { Hike } from '../../types';
import { StatTile } from '../dashboard/parts';
import { HorizontalBar } from '../charts/HorizontalBar';
import { useChartTheme } from '../../lib/useChartTheme';

// A titled analytics panel — matches the card style used across the app.
function ChartCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div className="text-sm font-semibold text-gray-900 dark:text-gray-100">{title}</div>
      {subtitle && <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{subtitle}</p>}
      <div className="mt-3">{children}</div>
    </div>
  );
}

interface AreaDatum {
  label: string;
  value: number;
}

function AreaTooltip({
  active,
  payload,
}: {
  active?: boolean;
  payload?: { payload: AreaDatum }[];
}) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm dark:border-gray-700 dark:bg-gray-800">
      <div className="font-medium text-gray-900 dark:text-gray-100">
        {p.value} hike{p.value === 1 ? '' : 's'}
      </div>
      <div className="text-gray-400 dark:text-gray-500">{p.label}</div>
    </div>
  );
}

// Hikes-per-year as a filled trend, so the shape of your hiking history reads at
// a glance rather than as a stack of equal-looking bars.
function HikesTrend({ data }: { data: AreaDatum[] }) {
  const t = useChartTheme();
  if (data.length === 0) {
    return (
      <div className="flex h-[220px] items-center justify-center text-sm text-gray-400 dark:text-gray-500">
        No hikes in this range yet.
      </div>
    );
  }
  return (
    <ResponsiveContainer width="100%" height={220}>
      <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
        <defs>
          <linearGradient id="hikeTrendFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={t.indigo} stopOpacity={0.35} />
            <stop offset="100%" stopColor={t.indigo} stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" vertical={false} stroke={t.grid} />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 11, fill: t.axis }}
          tickLine={false}
          axisLine={false}
          minTickGap={4}
        />
        <YAxis
          tick={{ fontSize: 11, fill: t.axis }}
          tickLine={false}
          axisLine={false}
          width={28}
          allowDecimals={false}
        />
        <Tooltip content={<AreaTooltip />} cursor={{ stroke: t.cursorStroke, strokeWidth: 1 }} />
        <Area
          type="monotone"
          dataKey="value"
          stroke={t.indigo}
          strokeWidth={2}
          fill="url(#hikeTrendFill)"
          dot={{ r: 3, fill: t.indigo, strokeWidth: 0 }}
          activeDot={{ r: 5, stroke: t.activeDotStroke, strokeWidth: 2 }}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

interface Slice {
  name: string;
  value: number;
}

// Camped vs day-trip split as a donut — a part-to-whole shape the bar rows never
// showed. Center holds the headline (share of trips where you camped out).
function TripComposition({ camped, total }: { camped: number; total: number }) {
  const t = useChartTheme();
  const data: Slice[] = [
    { name: 'Camped', value: camped },
    { name: 'Day / no camp', value: Math.max(total - camped, 0) },
  ];
  const colors = [t.categorical[0], t.categorical[1]];
  const pct = total > 0 ? Math.round((camped / total) * 100) : 0;

  return (
    <div className="flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
      <div className="relative h-[180px] w-[180px] shrink-0">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={data}
              dataKey="value"
              nameKey="name"
              innerRadius={58}
              outerRadius={82}
              paddingAngle={2}
              stroke="none"
              startAngle={90}
              endAngle={-270}
              isAnimationActive={false}
            >
              {data.map((s, i) => (
                <Cell key={s.name} fill={colors[i]} />
              ))}
            </Pie>
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-2xl font-semibold tabular-nums text-gray-900 dark:text-gray-50">
            {pct}%
          </span>
          <span className="text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
            camped
          </span>
        </div>
      </div>
      <ul className="space-y-2">
        {data.map((s, i) => (
          <li key={s.name} className="flex items-center gap-2 text-sm">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: colors[i] }}
            />
            <span className="text-gray-600 dark:text-gray-300">{s.name}</span>
            <span className="ml-auto pl-4 font-medium tabular-nums text-gray-900 dark:text-gray-100">
              {s.value}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function HikeAnalytics({ hikes }: { hikes: Hike[] }) {
  const stats = useMemo(() => {
    const mountains = new Map<string, number>();
    const companions = new Map<string, number>();
    const years = new Map<number, number>();
    let days = 0;
    let nights = 0;
    let camped = 0;
    const currentYear = new Date().getFullYear();
    let thisYear = 0;

    for (const h of hikes) {
      days += h.days || 0;
      nights += h.nights || 0;
      if (h.camped) camped += 1;
      if (h.mountain) mountains.set(h.mountain, (mountains.get(h.mountain) ?? 0) + 1);
      for (const p of h.participants) companions.set(p, (companions.get(p) ?? 0) + 1);
      const y = Number(h.hiked_on.slice(0, 4));
      if (!Number.isNaN(y) && y > 0) {
        years.set(y, (years.get(y) ?? 0) + 1);
        if (y === currentYear) thisYear += 1;
      }
    }

    const rank = (m: Map<string, number>, n: number) =>
      [...m.entries()]
        .map(([name, count]) => ({ name, count }))
        .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
        .slice(0, n);

    const topMountains = rank(mountains, 6);
    const topCompanions = rank(companions, 6);
    // Ascending by year so the trend reads left→old to right→recent.
    const byYear = [...years.entries()]
      .map(([year, count]) => ({ year, count }))
      .sort((a, b) => a.year - b.year);

    return {
      total: hikes.length,
      peaks: mountains.size,
      days,
      nights,
      camped,
      companions: companions.size,
      thisYear,
      topMountains,
      topCompanions,
      byYear,
    };
  }, [hikes]);

  const trend: AreaDatum[] = stats.byYear.map((y) => ({ label: String(y.year), value: y.count }));
  const peakBars = stats.topMountains.map((m) => ({
    name: m.name,
    value: m.count,
    display: `${m.count} hike${m.count === 1 ? '' : 's'}`,
  }));
  const companionBars = stats.topCompanions.map((c) => ({
    name: c.name,
    value: c.count,
    display: `${c.count} trip${c.count === 1 ? '' : 's'}`,
  }));

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <StatTile
          label="Hikes"
          value={String(stats.total)}
          sub={stats.thisYear > 0 ? `${stats.thisYear} this year` : undefined}
        />
        <StatTile
          label="Peaks"
          value={String(stats.peaks)}
          sub={stats.topMountains[0] ? `Top: ${stats.topMountains[0].name}` : undefined}
        />
        <StatTile
          label="Days on trail"
          value={String(stats.days)}
          sub={stats.nights > 0 ? `${stats.nights} nights out` : undefined}
        />
        <StatTile
          label="Camped trips"
          value={String(stats.camped)}
          sub={
            stats.total > 0
              ? `${Math.round((stats.camped / stats.total) * 100)}% of hikes`
              : undefined
          }
        />
        <StatTile
          label="Companions"
          value={String(stats.companions)}
          sub={stats.topCompanions[0] ? `Most: ${stats.topCompanions[0].name}` : undefined}
        />
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <ChartCard title="Hikes over time" subtitle="Trips logged each year.">
          <HikesTrend data={trend} />
        </ChartCard>
        <ChartCard title="Trip composition" subtitle="How often you camped out versus day trips.">
          <div className="flex min-h-[220px] items-center justify-center">
            {stats.total > 0 ? (
              <TripComposition camped={stats.camped} total={stats.total} />
            ) : (
              <span className="text-sm text-gray-400 dark:text-gray-500">
                No hikes in this range yet.
              </span>
            )}
          </div>
        </ChartCard>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        {peakBars.length > 0 && (
          <ChartCard title="Most-climbed peaks" subtitle="Mountains you return to most.">
            <HorizontalBar data={peakBars} />
          </ChartCard>
        )}
        {companionBars.length > 0 && (
          <ChartCard title="Top companions" subtitle="Who’s joined you on the most trips.">
            <HorizontalBar data={companionBars} />
          </ChartCard>
        )}
      </div>
    </div>
  );
}
