import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts';
import { formatTokens } from '../../lib/format';

interface Point {
  label: string;
  value: number;
}

interface TooltipEntry {
  payload: Point;
}

function ChartTooltip({ active, payload }: { active?: boolean; payload?: TooltipEntry[] }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium text-gray-900">{formatTokens(p.value)} tokens</div>
      <div className="text-gray-400">{p.label}</div>
    </div>
  );
}

export function UsageAreaChart({ data }: { data: Point[] }) {
  if (data.length === 0) {
    return (
      <div className="flex h-[260px] items-center justify-center text-sm text-gray-400">
        No usage in this range yet.
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={260}>
      <AreaChart data={data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
        <defs>
          <linearGradient id="tokFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#4f46e5" stopOpacity={0.18} />
            <stop offset="100%" stopColor="#4f46e5" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#e5e7eb" />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 11, fill: '#9ca3af' }}
          tickLine={false}
          axisLine={false}
          minTickGap={28}
        />
        <YAxis
          tick={{ fontSize: 11, fill: '#9ca3af' }}
          tickLine={false}
          axisLine={false}
          width={48}
          tickFormatter={formatTokens}
        />
        <Tooltip content={<ChartTooltip />} cursor={{ stroke: '#c7c7c7', strokeWidth: 1 }} />
        <Area
          type="monotone"
          dataKey="value"
          stroke="#4f46e5"
          strokeWidth={2}
          fill="url(#tokFill)"
          activeDot={{ r: 4, fill: '#4f46e5', stroke: '#fff', strokeWidth: 2 }}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}
