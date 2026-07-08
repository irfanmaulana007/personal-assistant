import { ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';
import { formatTokens, formatCost } from '../../lib/format';

export interface ModelDatum {
  model: string;
  tokens: number;
  cost: number;
  rateKnown: boolean;
}

interface TooltipEntry {
  payload: ModelDatum;
}

function ChartTooltip({ active, payload }: { active?: boolean; payload?: TooltipEntry[] }) {
  if (!active || !payload?.length) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium text-gray-900">{p.model}</div>
      <div className="text-gray-500">{formatTokens(p.tokens)} tokens</div>
      <div className="text-gray-500">{p.rateKnown ? `${formatCost(p.cost)} est.` : 'no rate'}</div>
    </div>
  );
}

export function ModelBarChart({ data }: { data: ModelDatum[] }) {
  if (data.length === 0) {
    return <p className="text-sm text-gray-400">No data yet.</p>;
  }

  return (
    <ResponsiveContainer width="100%" height={Math.max(120, data.length * 48)}>
      <BarChart data={data} layout="vertical" margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
        <CartesianGrid horizontal={false} stroke="#e5e7eb" />
        <XAxis
          type="number"
          tickFormatter={formatTokens}
          tick={{ fontSize: 11, fill: '#9ca3af' }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          type="category"
          dataKey="model"
          width={140}
          tick={{ fontSize: 12, fill: '#374151' }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={<ChartTooltip />} cursor={{ fill: '#f3f4f6' }} />
        <Bar dataKey="tokens" fill="#4f46e5" radius={[0, 4, 4, 0]} barSize={18} />
      </BarChart>
    </ResponsiveContainer>
  );
}
