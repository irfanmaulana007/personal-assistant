import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts';

export interface LineSeries {
  key: string;
  name: string;
  color: string;
}

type Row = { label: string } & Record<string, number | string>;

interface PayloadEntry {
  name: string;
  value: number;
  color: string;
  dataKey: string;
}

function MLTooltip({
  active,
  payload,
  label,
  format,
}: {
  active?: boolean;
  payload?: PayloadEntry[];
  label?: string;
  format: (v: number) => string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm">
      <div className="mb-0.5 text-gray-400">{label}</div>
      {payload.map((p) => (
        <div key={p.dataKey} className="font-medium" style={{ color: p.color }}>
          {p.name}: {format(p.value)}
        </div>
      ))}
    </div>
  );
}

export function MultiLineChart({
  data,
  series,
  format = (v) => String(v),
}: {
  data: Row[];
  series: LineSeries[];
  format?: (v: number) => string;
}) {
  if (data.length === 0) {
    return (
      <div className="flex h-[240px] items-center justify-center text-sm text-gray-400">
        No data in this range yet.
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
          width={48}
          allowDecimals={false}
          tickFormatter={format}
        />
        <Tooltip
          content={<MLTooltip format={format} />}
          cursor={{ stroke: '#c7c7c7', strokeWidth: 1 }}
        />
        <Legend
          verticalAlign="top"
          height={28}
          iconType="plainline"
          wrapperStyle={{ fontSize: 12 }}
        />
        {series.map((sr) => (
          <Line
            key={sr.key}
            type="monotone"
            dataKey={sr.key}
            name={sr.name}
            stroke={sr.color}
            strokeWidth={2}
            dot={false}
            isAnimationActive={false}
            activeDot={{ r: 4, fill: sr.color, stroke: '#fff', strokeWidth: 2 }}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
}
