import { ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';
import { useChartTheme } from '../../lib/useChartTheme';

interface Datum {
  label: string;
  value: number;
}

interface TooltipEntry {
  payload: Datum;
}

function VBTooltip({ active, payload }: { active?: boolean; payload?: TooltipEntry[] }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm dark:border-gray-700 dark:bg-gray-800">
      <div className="font-medium text-gray-900 dark:text-gray-100">{p.value.toLocaleString()}</div>
      <div className="text-gray-400 dark:text-gray-500">{p.label}</div>
    </div>
  );
}

export function VerticalBar({ data, color }: { data: Datum[]; color?: string }) {
  const t = useChartTheme();
  const barColor = color ?? t.indigo;

  if (data.length === 0 || data.every((d) => d.value === 0)) {
    return (
      <div className="flex h-[200px] items-center justify-center text-sm text-gray-400 dark:text-gray-500">
        No data in this range yet.
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={200}>
      <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
        <CartesianGrid strokeDasharray="3 3" vertical={false} stroke={t.grid} />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 10, fill: t.axis }}
          tickLine={false}
          axisLine={false}
          interval="preserveStartEnd"
          minTickGap={4}
        />
        <YAxis
          tick={{ fontSize: 11, fill: t.axis }}
          tickLine={false}
          axisLine={false}
          width={36}
          allowDecimals={false}
        />
        <Tooltip content={<VBTooltip />} cursor={{ fill: t.cursorFill }} />
        <Bar dataKey="value" fill={barColor} radius={[3, 3, 0, 0]} isAnimationActive={false} />
      </BarChart>
    </ResponsiveContainer>
  );
}
