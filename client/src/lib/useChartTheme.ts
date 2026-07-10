import { useSyncExternalStore } from 'react';

// Recharts colours are passed as literal hex/rgba strings to SVG elements, so
// they can't use Tailwind's `dark:` variant. This hook tracks the `dark` class
// on <html> (toggled by src/lib/theme.ts) and hands back a palette that flips
// with the theme. Charts re-render when the class changes.

function subscribe(cb: () => void): () => void {
  const observer = new MutationObserver(cb);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  return () => observer.disconnect();
}

function isDark(): boolean {
  return document.documentElement.classList.contains('dark');
}

export interface ChartTheme {
  dark: boolean;
  grid: string;
  axis: string;
  axisStrong: string;
  cursorFill: string;
  cursorStroke: string;
  activeDotStroke: string;
  indigo: string;
  emerald: string;
}

const LIGHT: ChartTheme = {
  dark: false,
  grid: '#e5e7eb', // gray-200
  axis: '#9ca3af', // gray-400
  axisStrong: '#374151', // gray-700
  cursorFill: '#f3f4f6', // gray-100
  cursorStroke: '#c7c7c7',
  activeDotStroke: '#ffffff',
  indigo: '#4f46e5', // indigo-600
  emerald: '#059669', // emerald-600
};

const DARK: ChartTheme = {
  dark: true,
  grid: '#1f2937', // gray-800
  axis: '#9ca3af', // gray-400 — readable on dark too
  axisStrong: '#d1d5db', // gray-300
  cursorFill: 'rgba(255,255,255,0.06)',
  cursorStroke: '#4b5563', // gray-600
  activeDotStroke: '#1f2937', // match card surface (gray-800)
  indigo: '#818cf8', // indigo-400
  emerald: '#34d399', // emerald-400
};

export function useChartTheme(): ChartTheme {
  const dark = useSyncExternalStore(subscribe, isDark, () => false);
  return dark ? DARK : LIGHT;
}

/** Tracks the `dark` class on <html> for SVG fills that can't use Tailwind's
 *  `dark:` variant (colors passed as literal attributes). */
export function useIsDark(): boolean {
  return useSyncExternalStore(subscribe, isDark, () => false);
}
