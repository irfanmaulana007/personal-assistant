import { useState } from 'react';
import * as Popover from '@radix-ui/react-popover';
import { DayPicker, type DateRange } from 'react-day-picker';
import { format, parseISO, subDays, startOfMonth, endOfMonth, subMonths } from 'date-fns';
import 'react-day-picker/style.css';

interface DateRangePickerProps {
  from: string; // YYYY-MM-DD (inclusive) — empty string when no range is set
  to: string; // YYYY-MM-DD (inclusive) — empty string when no range is set
  onChange: (from: string, to: string) => void;
  // When true the control has an "all dates" (unset) state: it shows the
  // placeholder, exposes a clear affordance, and calls onChange('', '') to
  // clear. Existing callers that always pass a real range simply omit this.
  clearable?: boolean;
  placeholder?: string;
}

const iso = (d: Date) => format(d, 'yyyy-MM-dd');

// Compact, app-scale styling for react-day-picker (its defaults are oversized).
const rdpStyle = {
  '--rdp-accent-color': '#4f46e5',
  '--rdp-day-width': '2rem',
  '--rdp-day-height': '2rem',
  '--rdp-day_button-width': '2rem',
  '--rdp-day_button-height': '2rem',
  '--rdp-day_button-border-radius': '0.5rem',
  '--rdp-selected-border': '0',
  '--rdp-today-color': '#4f46e5',
  '--rdp-font-family': 'inherit',
  '--rdp-weekday-text-transform': 'none',
  '--rdp-nav_button-width': '1.75rem',
  '--rdp-nav_button-height': '1.75rem',
  fontSize: '0.8rem',
} as React.CSSProperties;

interface Preset {
  label: string;
  range: () => { from: Date; to: Date };
}

const presets: Preset[] = [
  { label: 'Today', range: () => ({ from: new Date(), to: new Date() }) },
  { label: 'Last 7 days', range: () => ({ from: subDays(new Date(), 6), to: new Date() }) },
  { label: 'Last 30 days', range: () => ({ from: subDays(new Date(), 29), to: new Date() }) },
  { label: 'Last 90 days', range: () => ({ from: subDays(new Date(), 89), to: new Date() }) },
  {
    label: 'This month',
    range: () => ({ from: startOfMonth(new Date()), to: new Date() }),
  },
  {
    label: 'Last month',
    range: () => ({
      from: startOfMonth(subMonths(new Date(), 1)),
      to: endOfMonth(subMonths(new Date(), 1)),
    }),
  },
];

function labelFor(from: string, to: string): string {
  const f = parseISO(from);
  const t = parseISO(to);
  if (from === to) return format(f, 'MMM d, yyyy');
  const sameYear = f.getFullYear() === t.getFullYear();
  return `${format(f, sameYear ? 'MMM d' : 'MMM d, yyyy')} – ${format(t, 'MMM d, yyyy')}`;
}

export function DateRangePicker({
  from,
  to,
  onChange,
  clearable = false,
  placeholder = 'All dates',
}: DateRangePickerProps) {
  const hasRange = Boolean(from && to);
  const initialDraft = (): DateRange | undefined =>
    hasRange ? { from: parseISO(from), to: parseISO(to) } : undefined;

  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<DateRange | undefined>(initialDraft);

  const applyPreset = (p: Preset) => {
    const r = p.range();
    setDraft(r);
    onChange(iso(r.from), iso(r.to));
    setOpen(false);
  };

  const applyDraft = () => {
    if (draft?.from && draft?.to) {
      onChange(iso(draft.from), iso(draft.to));
      setOpen(false);
    }
  };

  const clear = () => {
    setDraft(undefined);
    onChange('', '');
    setOpen(false);
  };

  const activePreset = presets.find((p) => {
    const r = p.range();
    return iso(r.from) === from && iso(r.to) === to;
  });

  return (
    <div className="inline-flex items-center">
      <Popover.Root
        open={open}
        onOpenChange={(o) => {
          setOpen(o);
          if (o) setDraft(initialDraft());
        }}
      >
        <Popover.Trigger asChild>
          <button
            className={`flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-medium transition hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:hover:bg-gray-800/60 ${
              hasRange ? 'text-gray-700 dark:text-gray-200' : 'text-gray-500 dark:text-gray-400'
            } ${clearable && hasRange ? 'rounded-r-none border-r-0' : ''}`}
          >
            <svg
              className="h-4 w-4 text-gray-400 dark:text-gray-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
              />
            </svg>
            {hasRange ? labelFor(from, to) : placeholder}
            <svg
              className="h-4 w-4 text-gray-400 dark:text-gray-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M19 9l-7 7-7-7"
              />
            </svg>
          </button>
        </Popover.Trigger>

        <Popover.Portal>
          <Popover.Content
            align="end"
            sideOffset={8}
            className="z-50 flex overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
          >
            <div className="w-36 shrink-0 border-r border-gray-100 p-1.5 dark:border-gray-800">
              {presets.map((p) => {
                const active = activePreset?.label === p.label;
                return (
                  <button
                    key={p.label}
                    onClick={() => applyPreset(p)}
                    className={`block w-full rounded-lg px-3 py-1.5 text-left text-[13px] font-medium transition ${
                      active
                        ? 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
                        : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'
                    }`}
                  >
                    {p.label}
                  </button>
                );
              })}
            </div>

            <div className="p-3 text-gray-700 dark:text-gray-200">
              <DayPicker
                mode="range"
                weekStartsOn={1}
                selected={draft}
                onSelect={setDraft}
                numberOfMonths={2}
                defaultMonth={hasRange ? parseISO(from) : new Date()}
                captionLayout="label"
                showOutsideDays={false}
                className="rdp-theme"
                style={rdpStyle}
              />
              <div className="mt-1 flex items-center justify-between gap-2 border-t border-gray-100 pt-2.5 dark:border-gray-800">
                <span className="text-xs text-gray-400 dark:text-gray-500">
                  {draft?.from && draft?.to
                    ? `${format(draft.from, 'MMM d')} – ${format(draft.to, 'MMM d, yyyy')}`
                    : 'Pick a start and end date'}
                </span>
                <div className="flex gap-2">
                  {clearable && (
                    <button
                      onClick={clear}
                      className="rounded-lg px-3 py-1.5 text-[13px] font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
                    >
                      Clear
                    </button>
                  )}
                  <button
                    onClick={() => setOpen(false)}
                    className="rounded-lg px-3 py-1.5 text-[13px] font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={applyDraft}
                    disabled={!draft?.from || !draft?.to}
                    className="rounded-lg bg-indigo-600 px-3 py-1.5 text-[13px] font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
                  >
                    Apply
                  </button>
                </div>
              </div>
            </div>
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>

      {clearable && hasRange && (
        <button
          type="button"
          aria-label="Clear date filter"
          onClick={clear}
          className="flex items-center rounded-xl rounded-l-none border border-gray-200 bg-white px-2 py-2 text-gray-400 transition hover:bg-gray-50 hover:text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-500 dark:hover:bg-gray-800/60 dark:hover:text-gray-300"
        >
          <svg
            className="h-4 w-4"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 6l12 12M18 6L6 18" />
          </svg>
        </button>
      )}
    </div>
  );
}
