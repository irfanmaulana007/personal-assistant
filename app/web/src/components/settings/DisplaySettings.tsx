import { useState } from 'react';
import { updatePreferences } from '../../api/client';
import { usePreferences } from '../../contexts/preferences';
import type { Preferences } from '../../types';
import { getStoredTheme, setTheme, type ThemeChoice } from '../../lib/theme';

const inputClass =
  'w-full rounded-xl border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

const THEME_OPTIONS: { value: ThemeChoice; label: string; icon: React.ReactNode }[] = [
  {
    value: 'light',
    label: 'Light',
    icon: (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        className="h-4 w-4"
      >
        <circle cx="12" cy="12" r="4" />
        <path
          strokeLinecap="round"
          d="M12 2v2m0 16v2M4.93 4.93l1.41 1.41m11.32 11.32 1.41 1.41M2 12h2m16 0h2M4.93 19.07l1.41-1.41m11.32-11.32 1.41-1.41"
        />
      </svg>
    ),
  },
  {
    value: 'dark',
    label: 'Dark',
    icon: (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        className="h-4 w-4"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79Z"
        />
      </svg>
    ),
  },
  {
    value: 'auto',
    label: 'System',
    icon: (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        className="h-4 w-4"
      >
        <rect x="2" y="4" width="20" height="13" rx="2" />
        <path strokeLinecap="round" d="M8 21h8m-4-4v4" />
      </svg>
    ),
  },
];

function ThemeControl() {
  const [theme, setThemeState] = useState<ThemeChoice>(() => getStoredTheme());

  const choose = (value: ThemeChoice) => {
    setThemeState(value);
    setTheme(value); // persists to localStorage + applies instantly
  };

  return (
    <div
      role="radiogroup"
      aria-label="Theme"
      className="inline-flex gap-1 rounded-xl border border-gray-200 bg-gray-50 p-1 dark:border-gray-700 dark:bg-gray-900"
    >
      {THEME_OPTIONS.map((opt) => {
        const active = theme === opt.value;
        return (
          <button
            key={opt.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => choose(opt.value)}
            className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition ${
              active
                ? 'bg-white text-indigo-700 shadow-sm dark:bg-gray-700 dark:text-indigo-300'
                : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'
            }`}
          >
            {opt.icon}
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

export function DisplaySettings() {
  const { prefs, reload } = usePreferences();

  const [timezone, setTimezone] = useState<string | null>(null);
  const [currency, setCurrency] = useState<string | null>(null);
  const [rate, setRate] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const tzVal = timezone ?? prefs.timezone;
  const curVal = currency ?? prefs.currency;
  const rateVal = rate ?? String(prefs.usd_to_idr);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setMsg(null);
    const payload: Preferences = {
      timezone: tzVal,
      currency: curVal,
      usd_to_idr: Number(rateVal) || prefs.usd_to_idr,
    };
    try {
      await updatePreferences(payload);
      reload();
      setTimezone(null);
      setCurrency(null);
      setRate(null);
      setMsg({ ok: true, text: 'Display preferences saved.' });
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to save' });
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={save}
      className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800"
    >
      <h2 className="mb-1 text-base font-semibold text-gray-900 dark:text-gray-50">Display</h2>
      <p className="mb-5 text-sm text-gray-500 dark:text-gray-400">
        How the app looks and how dates and amounts are shown across the dashboard, logs, and
        everywhere else.
      </p>

      <div className="space-y-4">
        <div>
          <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Theme
          </label>
          <ThemeControl />
          <p className="mt-1.5 text-xs text-gray-400 dark:text-gray-500">
            “System” follows your device’s light or dark setting. Saved on this device.
          </p>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Time offset
          </label>
          <select
            value={tzVal}
            onChange={(e) => setTimezone(e.target.value)}
            className={inputClass}
          >
            <option value="UTC">UTC</option>
            <option value="Asia/Jakarta">GMT+7 (WIB)</option>
          </select>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Currency
          </label>
          <select
            value={curVal}
            onChange={(e) => setCurrency(e.target.value)}
            className={inputClass}
          >
            <option value="USD">USD ($)</option>
            <option value="IDR">IDR (Rp)</option>
          </select>
        </div>

        {curVal === 'IDR' && (
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              USD → IDR rate
            </label>
            <input
              type="number"
              min={1}
              step={1}
              value={rateVal}
              onChange={(e) => setRate(e.target.value)}
              className={inputClass}
            />
            <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
              Used to convert estimated cost for display. 1 USD = {Number(rateVal).toLocaleString()}{' '}
              IDR.
            </p>
          </div>
        )}
      </div>

      <div className="mt-5 rounded-xl bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:bg-gray-900 dark:text-gray-300">
        <div className="text-[11px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
          Preview
        </div>
        <div className="mt-1">
          {new Intl.DateTimeFormat('en-US', {
            timeZone: tzVal,
            month: 'short',
            day: 'numeric',
            year: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
            hour12: false,
          }).format(new Date())}{' '}
          ·{' '}
          {curVal === 'IDR'
            ? `Rp${Math.round(1.2345 * (Number(rateVal) || 16000)).toLocaleString('id-ID')}`
            : '$1.23'}
        </div>
      </div>

      {msg && (
        <p
          className={`mt-4 text-sm ${
            msg.ok ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'
          }`}
        >
          {msg.text}
        </p>
      )}

      <button
        type="submit"
        disabled={saving}
        className="mt-5 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
      >
        {saving ? 'Saving…' : 'Save'}
      </button>
    </form>
  );
}
