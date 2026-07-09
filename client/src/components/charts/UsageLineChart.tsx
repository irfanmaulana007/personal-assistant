import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts';

interface Point {
  label: string;
  value: number;
}

interface TooltipEntry {
  payload: Point;
}

function LineTooltip({
  active,
  payload,
  format,
  unit = '',
}: {
  active?: boolean;
  payload?: TooltipEntry[];
  format: (v: number) => string;
  unit?: string;
}) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium text-gray-900">
        {format(p.value)}
        {unit}
      </div>
      <div className="text-gray-400">{p.label}</div>
    </div>
  );
}

export function UsageLineChart({
  data,
  color = '#4f46e5',
  format,
  unit = '',
}: {
  data: Point[];
  color?: string;
  format: (v: number) => string;
  unit?: string;
}) {
  if (data.length === 0) {
    return (
      <div className="flex h-[240px] items-center justify-center text-sm text-gray-400">
        No usage in this range yet.
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
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
          width={52}
          tickFormatter={format}
        />
        <Tooltip
          content={<LineTooltip format={format} unit={unit} />}
          cursor={{ stroke: '#c7c7c7', strokeWidth: 1 }}
        />
        <Line
          type="monotone"
          dataKey="value"
          stroke={color}
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
          activeDot={{ r: 4, fill: color, stroke: '#fff', strokeWidth: 2 }}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
