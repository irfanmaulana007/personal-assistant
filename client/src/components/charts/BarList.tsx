interface BarItem {
  label: string;
  value: number;
  meta?: string;
}

interface BarListProps {
  items: BarItem[];
  formatValue?: (n: number) => string;
}

export function BarList({ items, formatValue = (n) => String(n) }: BarListProps) {
  if (items.length === 0) {
    return <p className="text-sm text-gray-400">No data yet.</p>;
  }
  const max = Math.max(...items.map((i) => i.value), 1);

  return (
    <div className="space-y-3">
      {items.map((item) => (
        <div key={item.label}>
          <div className="mb-1 flex items-baseline justify-between gap-2 text-sm">
            <span className="truncate font-medium text-gray-700">{item.label}</span>
            <span className="shrink-0 text-gray-500 tabular-nums">
              {formatValue(item.value)}
              {item.meta && <span className="ml-2 text-gray-400">{item.meta}</span>}
            </span>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-full bg-gray-100">
            <div
              className="h-full rounded-full bg-indigo-500"
              style={{ width: `${Math.max((item.value / max) * 100, 2)}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}
