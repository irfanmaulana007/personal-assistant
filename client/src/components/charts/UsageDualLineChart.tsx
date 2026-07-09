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
import { formatTokens } from '../../lib/format';

export interface DualPoint {
  label: string;
  tokens: number;
  cost: number;
}

interface TooltipEntry {
  payload: DualPoint;
}

function DualTooltip({
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
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm">
      <div className="mb-0.5 text-gray-400">{p.label}</div>
      <div className="font-medium text-indigo-600">{formatTokens(p.tokens)} tokens</div>
      <div className="font-medium text-emerald-600">{formatMoney(p.cost)}</div>
    </div>
  );
}

export function UsageDualLineChart({
  data,
  formatMoney,
}: {
  data: DualPoint[];
  formatMoney: (usd: number) => string;
}) {
  if (data.length === 0) {
    return (
      <div className="flex h-[280px] items-center justify-center text-sm text-gray-400">
        No usage in this range yet.
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={280}>
      <LineChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
        <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#e5e7eb" />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 11, fill: '#9ca3af' }}
          tickLine={false}
          axisLine={false}
          minTickGap={28}
        />
        <YAxis
          yAxisId="tokens"
          tick={{ fontSize: 11, fill: '#4f46e5' }}
          tickLine={false}
          axisLine={false}
          width={52}
          tickFormatter={formatTokens}
        />
        <YAxis
          yAxisId="cost"
          orientation="right"
          tick={{ fontSize: 11, fill: '#059669' }}
          tickLine={false}
          axisLine={false}
          width={64}
          tickFormatter={formatMoney}
        />
        <Tooltip
          content={<DualTooltip formatMoney={formatMoney} />}
          cursor={{ stroke: '#c7c7c7', strokeWidth: 1 }}
        />
        <Legend
          verticalAlign="top"
          height={28}
          iconType="plainline"
          wrapperStyle={{ fontSize: 12 }}
        />
        <Line
          yAxisId="tokens"
          type="monotone"
          dataKey="tokens"
          name="Tokens"
          stroke="#4f46e5"
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
          activeDot={{ r: 4, fill: '#4f46e5', stroke: '#fff', strokeWidth: 2 }}
        />
        <Line
          yAxisId="cost"
          type="monotone"
          dataKey="cost"
          name="Cost"
          stroke="#059669"
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
          activeDot={{ r: 4, fill: '#059669', stroke: '#fff', strokeWidth: 2 }}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
