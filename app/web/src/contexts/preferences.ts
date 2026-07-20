import { createContext, useContext } from 'react';
import type { Preferences } from '../types';

export const PREF_DEFAULTS: Preferences = { timezone: 'UTC', currency: 'USD', usd_to_idr: 16000 };

/** Shown wherever the assistant refers to itself when no name is configured. */
export const ASSISTANT_NAME_FALLBACK = 'Assistant';

export interface PreferencesValue {
  prefs: Preferences;
  /** Configured assistant name, or {@link ASSISTANT_NAME_FALLBACK} when blank. */
  assistantName: string;
  reload: () => void;
  /** Format an ISO timestamp in the selected timezone. */
  formatDate: (iso: string, opts?: { time?: boolean; seconds?: boolean }) => string;
  /** Format a USD amount in the selected currency. */
  formatMoney: (usd: number) => string;
  /**
   * Compact chat timestamp in the selected timezone: time if today,
   * "Yesterday" if yesterday, weekday name if earlier this week (Mon-start),
   * otherwise the date.
   */
  formatChatTime: (iso: string) => string;
}

export const PreferencesCtx = createContext<PreferencesValue | null>(null);

export function usePreferences(): PreferencesValue {
  const c = useContext(PreferencesCtx);
  if (!c) throw new Error('usePreferences must be used within PreferencesProvider');
  return c;
}
