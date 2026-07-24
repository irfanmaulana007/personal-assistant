import { useMemo, useState } from 'react';
import {
  addDays,
  eachDayOfInterval,
  endOfMonth,
  endOfWeek,
  format,
  isSameDay,
  isSameMonth,
  parseISO,
  startOfMonth,
  startOfWeek,
} from 'date-fns';
import type { Hike } from '../../types';
import { Modal } from '../ui/Modal';

const WEEKDAYS = ['S', 'M', 'T', 'W', 'T', 'F', 'S'];
const MONTHS = Array.from({ length: 12 }, (_, i) => i);

function currentYear(): number {
  return new Date().getFullYear();
}

// The most recent year that has a dated hike, so the calendar opens on the
// year the user most likely wants to see. Falls back to the current year.
function latestHikeYear(hikes: Hike[]): number {
  let latest = '';
  for (const h of hikes) {
    if (h.hiked_on && h.hiked_on > latest) latest = h.hiked_on;
  }
  return latest ? Number(latest.slice(0, 4)) : currentYear();
}

export function HikeCalendarModal({
  open,
  hikes,
  onClose,
  onSelectHike,
}: {
  open: boolean;
  hikes: Hike[];
  onClose: () => void;
  onSelectHike: (h: Hike) => void;
}) {
  const [year, setYear] = useState(() => latestHikeYear(hikes));

  // Map every calendar day a hike covers → the hikes on that day. A multi-day
  // hike (days = N) spans hiked_on … hiked_on + (N - 1); undated hikes are
  // skipped. Rebuilt only when the hike set changes.
  const dayMap = useMemo(() => {
    const map = new Map<string, Hike[]>();
    for (const h of hikes) {
      if (!h.hiked_on) continue;
      const start = parseISO(h.hiked_on);
      if (isNaN(start.getTime())) continue;
      const span = Math.max(1, h.days || 1);
      for (let i = 0; i < span; i++) {
        const key = format(addDays(start, i), 'yyyy-MM-dd');
        const list = map.get(key);
        if (list) list.push(h);
        else map.set(key, [h]);
      }
    }
    return map;
  }, [hikes]);

  // Number of distinct calendar days marked within the selected year — a small
  // "you hiked N days" summary in the header.
  const daysHikedThisYear = useMemo(() => {
    const prefix = String(year);
    let count = 0;
    for (const key of dayMap.keys()) {
      if (key.startsWith(prefix)) count++;
    }
    return count;
  }, [dayMap, year]);

  const today = new Date();

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Hike calendar"
      description="Every date you’ve hiked, across the year. Click a marked date to open the trip."
      widthClass="max-w-5xl"
    >
      {open && (
        <div>
          {/* Year navigator + summary */}
          <div className="mb-5 flex items-center justify-between gap-4">
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setYear((y) => y - 1)}
                aria-label="Previous year"
                className="rounded-lg p-1.5 text-gray-500 transition hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
              >
                <svg
                  className="h-5 w-5"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15 6l-6 6 6 6" />
                </svg>
              </button>
              <div className="min-w-[4rem] text-center text-lg font-semibold tabular-nums text-gray-900 dark:text-gray-50">
                {year}
              </div>
              <button
                type="button"
                onClick={() => setYear((y) => y + 1)}
                aria-label="Next year"
                className="rounded-lg p-1.5 text-gray-500 transition hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
              >
                <svg
                  className="h-5 w-5"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 6l6 6-6 6" />
                </svg>
              </button>
            </div>
            <div className="text-sm text-gray-500 dark:text-gray-400">
              {daysHikedThisYear > 0 ? (
                <>
                  <span className="font-medium text-gray-900 dark:text-gray-100">
                    {daysHikedThisYear}
                  </span>{' '}
                  day{daysHikedThisYear === 1 ? '' : 's'} hiked in {year}
                </>
              ) : (
                <>No hikes recorded in {year}</>
              )}
            </div>
          </div>

          {/* 12 month mini-calendars */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {MONTHS.map((month) => (
              <MonthGrid
                key={month}
                year={year}
                month={month}
                dayMap={dayMap}
                today={today}
                onSelectHike={onSelectHike}
              />
            ))}
          </div>

          {/* Legend */}
          <div className="mt-5 flex flex-wrap items-center gap-x-4 gap-y-1.5 border-t border-gray-200 pt-4 text-xs text-gray-500 dark:border-gray-700 dark:text-gray-400">
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-flex h-4 w-4 items-center justify-center rounded-md bg-indigo-600 text-[9px] font-semibold text-white dark:bg-indigo-500">
                2
              </span>
              Hiked date — number is the trip’s day count
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-flex h-4 w-4 rounded-md ring-1 ring-inset ring-indigo-400 dark:ring-indigo-400" />
              Today
            </span>
          </div>
        </div>
      )}
    </Modal>
  );
}

function MonthGrid({
  year,
  month,
  dayMap,
  today,
  onSelectHike,
}: {
  year: number;
  month: number;
  dayMap: Map<string, Hike[]>;
  today: Date;
  onSelectHike: (h: Hike) => void;
}) {
  const first = new Date(year, month, 1);
  const monthStart = startOfMonth(first);
  const monthEnd = endOfMonth(first);
  const gridStart = startOfWeek(monthStart);
  const gridEnd = endOfWeek(monthEnd);
  const days = eachDayOfInterval({ start: gridStart, end: gridEnd });

  return (
    <div className="rounded-xl border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800">
      <div className="mb-2 px-0.5 text-sm font-semibold text-gray-900 dark:text-gray-50">
        {format(first, 'MMMM')}
      </div>
      <div className="grid grid-cols-7 gap-0.5">
        {WEEKDAYS.map((d, i) => (
          <div
            key={i}
            className="pb-1 text-center text-[10px] font-medium uppercase text-gray-400 dark:text-gray-500"
          >
            {d}
          </div>
        ))}
        {days.map((day) => {
          const inMonth = isSameMonth(day, monthStart);
          const key = format(day, 'yyyy-MM-dd');
          const dayHikes = inMonth ? dayMap.get(key) : undefined;
          const isToday = isSameDay(day, today);

          if (dayHikes && dayHikes.length > 0) {
            const longest = dayHikes.reduce((a, b) => (b.days > a.days ? b : a));
            const label = `${format(day, 'MMM d, yyyy')}\n${dayHikes
              .map((h) => `${h.mountain} (${h.days}D/${h.nights}N)`)
              .join('\n')}`;
            return (
              <button
                key={key}
                type="button"
                title={label}
                onClick={() => onSelectHike(dayHikes[0])}
                className="relative flex aspect-square items-center justify-center rounded-md bg-indigo-600 text-xs font-semibold text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
              >
                {format(day, 'd')}
                <span className="absolute -bottom-0.5 right-0.5 text-[8px] font-semibold leading-none text-indigo-100">
                  {longest.days}
                  {dayHikes.length > 1 ? `·${dayHikes.length}` : ''}
                </span>
              </button>
            );
          }

          return (
            <div
              key={key}
              className={[
                'flex aspect-square items-center justify-center rounded-md text-xs',
                inMonth ? 'text-gray-700 dark:text-gray-300' : 'text-gray-300 dark:text-gray-600',
                isToday && inMonth ? 'ring-1 ring-inset ring-indigo-400 dark:ring-indigo-400' : '',
              ].join(' ')}
            >
              {format(day, 'd')}
            </div>
          );
        })}
      </div>
    </div>
  );
}
