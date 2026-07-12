import { useState, type ReactNode } from 'react';
import * as Popover from '@radix-ui/react-popover';

export interface MultiSelectOption<T extends string> {
  value: T;
  label: string;
}

interface MultiSelectProps<T extends string> {
  /** Noun shown in the trigger when nothing is selected, e.g. "Channel". */
  label: string;
  options: MultiSelectOption<T>[];
  /** Currently selected values ([] = none, treated by callers as "all"). */
  value: T[];
  onChange: (next: T[]) => void;
  /** Optional leading icon rendered in the trigger. */
  icon?: ReactNode;
}

/**
 * A compact multi-select dropdown: a pill trigger that opens a checkbox list in
 * a Radix popover. An empty selection reads as "All {label}". Selecting values
 * narrows to that subset; toggling them all off returns to "All".
 *
 * Kept deliberately generic (string-valued) so every filter — channels, judge
 * scores, and any future dimension — shares one styled, themed control.
 */
export function MultiSelect<T extends string>({
  label,
  options,
  value,
  onChange,
  icon,
}: MultiSelectProps<T>) {
  const [open, setOpen] = useState(false);
  const selected = new Set(value);

  const toggle = (v: T) => {
    // Preserve `options` order so the URL param and summary stay stable.
    const next = options
      .map((o) => o.value)
      .filter((o) => (o === v ? !selected.has(o) : selected.has(o)));
    onChange(next);
  };

  const summary =
    value.length === 0
      ? `All ${label}`
      : options
          .filter((o) => selected.has(o.value))
          .map((o) => o.label)
          .join(', ');

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="flex max-w-[16rem] items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-800/60"
        >
          {icon && <span className="text-gray-400 dark:text-gray-500">{icon}</span>}
          <span className="truncate">{summary}</span>
          {value.length > 0 && (
            <span className="rounded-full bg-indigo-100 px-1.5 text-xs font-semibold tabular-nums text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300">
              {value.length}
            </span>
          )}
          <svg
            className="h-4 w-4 shrink-0 text-gray-400 dark:text-gray-500"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
      </Popover.Trigger>

      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={8}
          className="z-50 w-52 overflow-hidden rounded-2xl border border-gray-200 bg-white p-1.5 shadow-lg dark:border-gray-700 dark:bg-gray-800"
        >
          {options.map((o) => {
            const checked = selected.has(o.value);
            return (
              <button
                key={o.value}
                type="button"
                onClick={() => toggle(o.value)}
                className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-[13px] font-medium transition ${
                  checked
                    ? 'text-gray-900 dark:text-gray-50'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'
                }`}
              >
                <span
                  className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition ${
                    checked
                      ? 'border-indigo-600 bg-indigo-600 text-white dark:border-indigo-500 dark:bg-indigo-500'
                      : 'border-gray-300 bg-white dark:border-gray-600 dark:bg-gray-800'
                  }`}
                >
                  {checked && (
                    <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={3}
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                  )}
                </span>
                {o.label}
              </button>
            );
          })}

          {value.length > 0 && (
            <div className="mt-1 border-t border-gray-100 pt-1 dark:border-gray-800">
              <button
                type="button"
                onClick={() => onChange([])}
                className="w-full rounded-lg px-2.5 py-1.5 text-left text-[13px] font-medium text-gray-500 transition hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              >
                Clear
              </button>
            </div>
          )}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
