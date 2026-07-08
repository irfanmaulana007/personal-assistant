import { ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';

export interface HBarDatum {
  name: string;
  value: number;
  display: string; // preformatted value label, e.g. "12 calls"
}

interface TooltipEntry {
  payload: HBarDatum;
}

function ChartTooltip({ active, payload }: { active?: boolean; payload?: TooltipEntry[] }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium text-gray-900">{p.name}</div>
      <div className="text-gray-500">{p.display}</div>
    </div>
  );
}

interface HorizontalBarProps {
  data: HBarDatum[];
  format?: (n: number) => string;
}

export function HorizontalBar({ data, format = (n) => String(n) }: HorizontalBarProps) {
  if (data.length === 0) {
    return <p className="text-sm text-gray-400">No data yet.</p>;
  }

  return (
    <ResponsiveContainer width="100%" height={Math.max(120, data.length * 44)}>
      <BarChart data={data} layout="vertical" margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
        <CartesianGrid horizontal={false} stroke="#e5e7eb" />
        <XAxis
          type="number"
          allowDecimals={false}
          tickFormatter={format}
          tick={{ fontSize: 11, fill: '#9ca3af' }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          type="category"
          dataKey="name"
          width={140}
          tick={{ fontSize: 12, fill: '#374151' }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={<ChartTooltip />} cursor={{ fill: '#f3f4f6' }} />
        <Bar dataKey="value" fill="#4f46e5" radius={[0, 4, 4, 0]} barSize={16} />
      </BarChart>
    </ResponsiveContainer>
  );
}
