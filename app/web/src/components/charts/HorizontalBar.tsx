import { ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';
import { useChartTheme } from '../../lib/useChartTheme';

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
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm dark:border-gray-700 dark:bg-gray-800">
      <div className="font-medium text-gray-900 dark:text-gray-100">{p.name}</div>
      <div className="text-gray-500 dark:text-gray-400">{p.display}</div>
    </div>
  );
}

interface HorizontalBarProps {
  data: HBarDatum[];
  format?: (n: number) => string;
}

export function HorizontalBar({ data, format = (n) => String(n) }: HorizontalBarProps) {
  const t = useChartTheme();

  if (data.length === 0) {
    return <p className="text-sm text-gray-400 dark:text-gray-500">No data yet.</p>;
  }

  return (
    <ResponsiveContainer width="100%" height={Math.max(120, data.length * 44)}>
      <BarChart data={data} layout="vertical" margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
        <CartesianGrid horizontal={false} stroke={t.grid} />
        <XAxis
          type="number"
          allowDecimals={false}
          tickFormatter={format}
          tick={{ fontSize: 11, fill: t.axis }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          type="category"
          dataKey="name"
          width={140}
          tick={{ fontSize: 12, fill: t.axisStrong }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={<ChartTooltip />} cursor={{ fill: t.cursorFill }} />
        <Bar
          dataKey="value"
          fill={t.indigo}
          radius={[0, 4, 4, 0]}
          barSize={16}
          isAnimationActive={false}
        />
      </BarChart>
    </ResponsiveContainer>
  );
}
