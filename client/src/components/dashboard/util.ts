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
