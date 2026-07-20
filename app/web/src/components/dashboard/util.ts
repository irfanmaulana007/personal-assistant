import { useOutletContext } from 'react-router-dom';
import type { UsageStats } from '../../types';

export interface DashboardContext {
  stats: UsageStats;
}

export function useDashboard(): DashboardContext {
  return useOutletContext<DashboardContext>();
}

const MONTHS = [
  '',
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

export function formatDayLabel(iso: string): string {
  const [, m, d] = iso.split('-');
  return `${MONTHS[Number(m)]} ${Number(d)}`;
}

export function formatLatency(ms: number): string {
  if (!ms) return '—';
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${ms}ms`;
}

// Whole-hour offset from UTC for the two supported display timezones (both
// DST-free, so a simple rotation of the hourly buckets is exact).
export function tzOffsetHours(timezone: string): number {
  return timezone === 'Asia/Jakarta' ? 7 : 0;
}

// Rotate UTC hour buckets into local hours for the given offset.
export function hourlyData(byHour: number[], offset: number): { label: string; value: number }[] {
  return Array.from({ length: 24 }, (_, local) => ({
    label: String(local).padStart(2, '0'),
    value: byHour[(local - offset + 24) % 24] ?? 0,
  }));
}

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

export function weekdayData(byWeekday: number[]): { label: string; value: number }[] {
  return WEEKDAYS.map((label, i) => ({ label, value: byWeekday[i] ?? 0 }));
}
