import { useRef, useState } from 'react';

interface Point {
  label: string;
  value: number;
}

interface AreaChartProps {
  data: Point[];
  formatValue?: (n: number) => string;
  height?: number;
}

// Fixed viewBox coordinate space; the SVG scales to its container width.
const VB_W = 640;
const PAD = { top: 12, right: 12, bottom: 24, left: 44 };

export function AreaChart({ data, formatValue = (n) => String(n), height = 220 }: AreaChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<number | null>(null);

  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center text-sm text-gray-400" style={{ height }}>
        No usage in this range yet.
      </div>
    );
  }

  const VB_H = height;
  const innerW = VB_W - PAD.left - PAD.right;
  const innerH = VB_H - PAD.top - PAD.bottom;
  const maxV = Math.max(...data.map((d) => d.value), 1);
  const n = data.length;

  const x = (i: number) => PAD.left + (n === 1 ? innerW / 2 : (i / (n - 1)) * innerW);
  const y = (v: number) => PAD.top + innerH - (v / maxV) * innerH;

  const linePath = data.map((d, i) => `${i === 0 ? 'M' : 'L'} ${x(i)} ${y(d.value)}`).join(' ');
  const areaPath =
    n === 1 ? '' : `${linePath} L ${x(n - 1)} ${PAD.top + innerH} L ${x(0)} ${PAD.top + innerH} Z`;

  // Y gridlines at 0, 50%, 100%.
  const yTicks = [0, 0.5, 1].map((f) => ({ v: maxV * f, yy: y(maxV * f) }));

  const handleMove = (e: React.MouseEvent<SVGSVGElement>) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const frac = (e.clientX - rect.left) / rect.width;
    const idx = Math.round(frac * (n - 1));
    setHover(Math.max(0, Math.min(n - 1, idx)));
  };

  return (
    <div className="relative">
      <svg
        ref={svgRef}
        viewBox={`0 0 ${VB_W} ${VB_H}`}
        className="w-full"
        style={{ height }}
        preserveAspectRatio="none"
        onMouseMove={handleMove}
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          <linearGradient id="areaFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#4f46e5" stopOpacity="0.18" />
            <stop offset="100%" stopColor="#4f46e5" stopOpacity="0" />
          </linearGradient>
        </defs>

        {/* gridlines + y labels */}
        {yTicks.map((t, i) => (
          <g key={i}>
            <line
              x1={PAD.left}
              y1={t.yy}
              x2={VB_W - PAD.right}
              y2={t.yy}
              stroke="#e5e7eb"
              strokeWidth={1}
              vectorEffect="non-scaling-stroke"
            />
            <text
              x={PAD.left - 8}
              y={t.yy + 3}
              textAnchor="end"
              className="fill-gray-400"
              style={{ fontSize: 10 }}
            >
              {formatValue(Math.round(t.v))}
            </text>
          </g>
        ))}

        {areaPath && <path d={areaPath} fill="url(#areaFill)" />}
        <path
          d={linePath}
          fill="none"
          stroke="#4f46e5"
          strokeWidth={2}
          strokeLinejoin="round"
          strokeLinecap="round"
          vectorEffect="non-scaling-stroke"
        />

        {/* hover marker + crosshair */}
        {hover !== null && (
          <g>
            <line
              x1={x(hover)}
              y1={PAD.top}
              x2={x(hover)}
              y2={PAD.top + innerH}
              stroke="#c7c7c7"
              strokeWidth={1}
              vectorEffect="non-scaling-stroke"
            />
            <circle
              cx={x(hover)}
              cy={y(data[hover].value)}
              r={4}
              fill="#4f46e5"
              stroke="#ffffff"
              strokeWidth={2}
              vectorEffect="non-scaling-stroke"
            />
          </g>
        )}

        {/* x labels: first, middle, last to avoid crowding */}
        {[0, Math.floor((n - 1) / 2), n - 1]
          .filter((v, i, a) => a.indexOf(v) === i)
          .map((i) => (
            <text
              key={i}
              x={x(i)}
              y={VB_H - 6}
              textAnchor={i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle'}
              className="fill-gray-400"
              style={{ fontSize: 10 }}
            >
              {data[i].label}
            </text>
          ))}
      </svg>

      {hover !== null && (
        <div
          className="pointer-events-none absolute -translate-x-1/2 rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs shadow-sm"
          style={{ left: `${(hover / Math.max(n - 1, 1)) * 100}%`, top: 0 }}
        >
          <div className="font-medium text-gray-900">{formatValue(data[hover].value)}</div>
          <div className="text-gray-400">{data[hover].label}</div>
        </div>
      )}
    </div>
  );
}
