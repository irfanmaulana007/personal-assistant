// A pill-style segmented control for switching between a few top-level views
// (e.g. the master catalog vs. the projects-comparison matrix). Styling mirrors
// the filter pills already used on the /skills page.
export interface SegmentedTab<T extends string> {
  key: T;
  label: string;
  count?: number;
}

export function SegmentedTabs<T extends string>({
  tabs,
  active,
  onChange,
}: {
  tabs: SegmentedTab<T>[];
  active: T;
  onChange: (key: T) => void;
}) {
  return (
    <div className="inline-flex flex-wrap gap-1 rounded-xl border border-gray-200 bg-white p-1 dark:border-gray-700 dark:bg-gray-800">
      {tabs.map((t) => {
        const on = active === t.key;
        return (
          <button
            key={t.key}
            type="button"
            onClick={() => onChange(t.key)}
            className={`inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition ${
              on
                ? 'bg-indigo-600 text-white dark:bg-indigo-500'
                : 'text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-100'
            }`}
          >
            {t.label}
            {typeof t.count === 'number' && (
              <span
                className={`inline-flex min-w-4 items-center justify-center rounded-full px-1 text-[11px] font-semibold ${
                  on
                    ? 'bg-white/20 text-white'
                    : 'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
                }`}
              >
                {t.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
