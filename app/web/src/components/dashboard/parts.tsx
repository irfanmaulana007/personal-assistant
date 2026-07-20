import type { ReactNode } from 'react';

export function StatTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div className="text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
        {label}
      </div>
      <div className="mt-1.5 text-2xl font-semibold tracking-tight text-gray-900 tabular-nums dark:text-gray-50">
        {value}
      </div>
      {sub && <div className="mt-1 text-xs text-gray-400 dark:text-gray-500">{sub}</div>}
    </div>
  );
}

export function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
      <h2 className="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-50">{title}</h2>
      {children}
    </div>
  );
}
