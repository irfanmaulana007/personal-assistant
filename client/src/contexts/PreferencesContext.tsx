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

  // Calendar date (YYYY-MM-DD) of a moment in the selected timezone.
  const tzDate = (d: Date) =>
    new Intl.DateTimeFormat('en-CA', {
      timeZone: tz,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(d);

  const formatChatTime: PreferencesValue['formatChatTime'] = (iso) => {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;

    const dayMs = 86400000;
    const msgCal = new Date(`${tzDate(d)}T00:00:00Z`);
    const todayCal = new Date(`${tzDate(new Date())}T00:00:00Z`);
    const diffDays = Math.round((todayCal.getTime() - msgCal.getTime()) / dayMs);

    if (diffDays <= 0) {
      // Today → time only.
      return new Intl.DateTimeFormat('en-US', {
        timeZone: tz,
        hour: '2-digit',
        minute: '2-digit',
        hour12: true,
      }).format(d);
    }
    if (diffDays === 1) return 'Yesterday';

    // Monday of the current week (Mon = 0).
    const daysSinceMonday = (todayCal.getUTCDay() + 6) % 7;
    const mondayCal = new Date(todayCal.getTime() - daysSinceMonday * dayMs);
    if (msgCal.getTime() >= mondayCal.getTime()) {
      // Earlier this week → weekday name.
      return new Intl.DateTimeFormat('en-US', { timeZone: tz, weekday: 'long' }).format(d);
    }
    // Older → date.
    return new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    }).format(d);
  };

  return (
    <PreferencesCtx.Provider
      value={{
        prefs,
        reload: () => setTick((t) => t + 1),
        formatDate,
        formatMoney,
        formatChatTime,
      }}
    >
      {children}
    </PreferencesCtx.Provider>
  );
}
