import { useState } from 'react';
import { updatePreferences } from '../../api/client';
import { usePreferences } from '../../contexts/preferences';
import type { Preferences } from '../../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

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
    <form onSubmit={save} className="rounded-2xl border border-gray-200 bg-white p-6">
      <h2 className="mb-1 text-base font-semibold text-gray-900">Display</h2>
      <p className="mb-5 text-sm text-gray-500">
        How dates and amounts are shown across the dashboard, logs, and everywhere else.
      </p>

      <div className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Time offset</label>
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
          <label className="mb-1 block text-sm font-medium text-gray-700">Currency</label>
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
            <label className="mb-1 block text-sm font-medium text-gray-700">USD → IDR rate</label>
            <input
              type="number"
              min={1}
              step={1}
              value={rateVal}
              onChange={(e) => setRate(e.target.value)}
              className={inputClass}
            />
            <p className="mt-1 text-xs text-gray-400">
              Used to convert estimated cost for display. 1 USD = {Number(rateVal).toLocaleString()}{' '}
              IDR.
            </p>
          </div>
        )}
      </div>

      <div className="mt-5 rounded-xl bg-gray-50 px-4 py-3 text-sm text-gray-600">
        <div className="text-[11px] font-medium uppercase tracking-wide text-gray-400">Preview</div>
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
        <p className={`mt-4 text-sm ${msg.ok ? 'text-green-600' : 'text-red-600'}`}>{msg.text}</p>
      )}

      <button
        type="submit"
        disabled={saving}
        className="mt-5 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50"
      >
        {saving ? 'Saving…' : 'Save'}
      </button>
    </form>
  );
}
