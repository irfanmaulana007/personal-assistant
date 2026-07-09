import { createContext, useContext } from 'react';
import type { Preferences } from '../types';

export const PREF_DEFAULTS: Preferences = { timezone: 'UTC', currency: 'USD', usd_to_idr: 16000 };

export interface PreferencesValue {
  prefs: Preferences;
  reload: () => void;
  /** Format an ISO timestamp in the selected timezone. */
  formatDate: (iso: string, opts?: { time?: boolean; seconds?: boolean }) => string;
  /** Format a USD amount in the selected currency. */
  formatMoney: (usd: number) => string;
}

export const PreferencesCtx = createContext<PreferencesValue | null>(null);

export function usePreferences(): PreferencesValue {
  const c = useContext(PreferencesCtx);
  if (!c) throw new Error('usePreferences must be used within PreferencesProvider');
  return c;
}
