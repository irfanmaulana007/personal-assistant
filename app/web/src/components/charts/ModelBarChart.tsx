import { ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';
import { formatTokens } from '../../lib/format';
import { useChartTheme } from '../../lib/useChartTheme';

export interface ModelDatum {
  model: string;
  tokens: number;
  cost: number;
  rateKnown: boolean;
}

interface TooltipEntry {
  payload: ModelDatum;
}

function ChartTooltip({
  active,
  payload,
  formatMoney,
}: {
  active?: boolean;
  payload?: TooltipEntry[];
  formatMoney: (usd: number) => string;
}) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm dark:border-gray-700 dark:bg-gray-800">
      <div className="font-medium text-gray-900 dark:text-gray-100">{p.model}</div>
      <div className="text-gray-500 dark:text-gray-400">{formatTokens(p.tokens)} tokens</div>
      <div className="text-gray-500 dark:text-gray-400">
        {p.rateKnown ? `${formatMoney(p.cost)} est.` : 'no rate'}
      </div>
    </div>
  );
}

export function ModelBarChart({
  data,
  formatMoney,
}: {
  data: ModelDatum[];
  formatMoney: (usd: number) => string;
}) {
  const t = useChartTheme();

  if (data.length === 0) {
    return <p className="text-sm text-gray-400 dark:text-gray-500">No data yet.</p>;
  }

  return (
    <ResponsiveContainer width="100%" height={Math.max(120, data.length * 48)}>
      <BarChart data={data} layout="vertical" margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
        <CartesianGrid horizontal={false} stroke={t.grid} />
        <XAxis
          type="number"
          tickFormatter={formatTokens}
          tick={{ fontSize: 11, fill: t.axis }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          type="category"
          dataKey="model"
          width={140}
          tick={{ fontSize: 12, fill: t.axisStrong }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip
          content={<ChartTooltip formatMoney={formatMoney} />}
          cursor={{ fill: t.cursorFill }}
        />
        <Bar
          dataKey="tokens"
          fill={t.indigo}
          radius={[0, 4, 4, 0]}
          barSize={18}
          isAnimationActive={false}
        />
      </BarChart>
    </ResponsiveContainer>
  );
}
