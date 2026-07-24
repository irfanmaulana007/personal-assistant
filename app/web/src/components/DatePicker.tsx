import { useState } from 'react';
import * as Popover from '@radix-ui/react-popover';
import { DayPicker, type Matcher } from 'react-day-picker';
import { format, parseISO } from 'date-fns';
import 'react-day-picker/style.css';

interface DatePickerProps {
  value: string; // YYYY-MM-DD ('' when unset)
  onChange: (value: string) => void;
  min?: string; // YYYY-MM-DD, earliest selectable date (inclusive)
  max?: string; // YYYY-MM-DD, latest selectable date (inclusive)
  placeholder?: string;
  autoFocus?: boolean;
}

const iso = (d: Date) => format(d, 'yyyy-MM-dd');

// Compact, app-scale styling for react-day-picker (its defaults are oversized).
// Kept in sync with DateRangePicker so both pickers read as one system.
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

export function DatePicker({
  value,
  onChange,
  min,
  max,
  placeholder = 'Select date',
  autoFocus,
}: DatePickerProps) {
  const [open, setOpen] = useState(false);

  const selected = value ? parseISO(value) : undefined;

  const disabled: Matcher[] = [];
  if (min) disabled.push({ before: parseISO(min) });
  if (max) disabled.push({ after: parseISO(max) });

  const handleSelect = (d: Date | undefined) => {
    if (!d) return;
    onChange(iso(d));
    setOpen(false);
  };

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          autoFocus={autoFocus}
          className="flex w-full max-w-[12rem] items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200 dark:hover:bg-gray-800/60 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30"
        >
          <svg
            className="h-4 w-4 shrink-0 text-gray-400 dark:text-gray-500"
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
          <span className={value ? '' : 'text-gray-400 dark:text-gray-500'}>
            {value ? format(selected as Date, 'MMM d, yyyy') : placeholder}
          </span>
          <svg
            className="ml-auto h-4 w-4 shrink-0 text-gray-400 dark:text-gray-500"
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
          align="start"
          sideOffset={8}
          className="z-50 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
        >
          <div className="p-3 text-gray-700 dark:text-gray-200">
            <DayPicker
              mode="single"
              weekStartsOn={1}
              selected={selected}
              onSelect={handleSelect}
              disabled={disabled.length ? disabled : undefined}
              defaultMonth={selected ?? (max ? parseISO(max) : undefined)}
              captionLayout="label"
              showOutsideDays={false}
              className="rdp-theme"
              style={rdpStyle}
            />
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
