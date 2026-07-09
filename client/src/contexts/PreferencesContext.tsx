import { useState, useEffect, type ReactNode } from 'react';
import { getPreferences } from '../api/client';
import type { Preferences } from '../types';
import { PreferencesCtx, PREF_DEFAULTS, type PreferencesValue } from './preferences';

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<Preferences>(PREF_DEFAULTS);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let active = true;
    getPreferences()
      .then((p) => active && setPrefs(p))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [tick]);

  const tz = prefs.timezone || 'UTC';

  const formatDate: PreferencesValue['formatDate'] = (iso, opts) => {
    if (!iso) return '—';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    const fmt: Intl.DateTimeFormatOptions = {
      timeZone: tz,
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    };
    if (opts?.time) {
      fmt.hour = '2-digit';
      fmt.minute = '2-digit';
      fmt.hour12 = false;
      if (opts.seconds) fmt.second = '2-digit';
    }
    return new Intl.DateTimeFormat('en-US', fmt).format(d);
  };

  const formatMoney: PreferencesValue['formatMoney'] = (usd) => {
    if (prefs.currency === 'IDR') {
      const idr = Math.round(usd * (prefs.usd_to_idr || PREF_DEFAULTS.usd_to_idr));
      return `Rp${idr.toLocaleString('id-ID')}`;
    }
    if (usd > 0 && usd < 0.01) return `$${usd.toFixed(4)}`;
    return `$${usd.toFixed(2)}`;
  };

  return (
    <PreferencesCtx.Provider
      value={{ prefs, reload: () => setTick((t) => t + 1), formatDate, formatMoney }}
    >
      {children}
    </PreferencesCtx.Provider>
  );
}
