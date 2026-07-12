import { useState, useEffect } from 'react';
import { getWhatsAppAllowlist, setWhatsAppAllowlist } from '../../api/client';
import { SkeletonFormCard } from '../ui/Skeleton';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

// Show a JID as the friendlier bare number in the input.
function toDisplay(jid: string): string {
  return jid.includes('@') ? jid.split('@')[0] : jid;
}

type Mode = 'all' | 'allowlist';

export function WhatsAppSettings() {
  const [mode, setMode] = useState<Mode>('allowlist');
  const [numbers, setNumbers] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    let active = true;
    getWhatsAppAllowlist()
      .then((d) => {
        if (!active) return;
        setMode(d.allow_all ? 'all' : 'allowlist');
        setNumbers(d.allowlist.length ? d.allowlist.map(toDisplay) : ['']);
      })
      .catch(() => {})
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const allowAll = mode === 'all';

  const setAt = (i: number, v: string) =>
    setNumbers((ns) => ns.map((n, idx) => (idx === i ? v : n)));
  const add = () => setNumbers((ns) => [...ns, '']);
  const removeAt = (i: number) => setNumbers((ns) => ns.filter((_, idx) => idx !== i));

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setMsg(null);
    try {
      const cleaned = numbers.map((n) => n.trim()).filter(Boolean);
      const d = await setWhatsAppAllowlist(cleaned, allowAll);
      setMode(d.allow_all ? 'all' : 'allowlist');
      setNumbers(d.allowlist.length ? d.allowlist.map(toDisplay) : ['']);
      setMsg({ ok: true, text: 'WhatsApp settings saved.' });
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to save' });
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <SkeletonFormCard fields={3} />;

  const segBase = 'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition focus:outline-none';
  const segActive = 'bg-white text-indigo-700 shadow-sm dark:bg-gray-700 dark:text-indigo-300';
  const segIdle = 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200';

  return (
    <form
      onSubmit={save}
      className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800"
    >
      <h2 className="mb-1 text-base font-semibold text-gray-900 dark:text-gray-50">
        WhatsApp agent access
      </h2>
      <p className="mb-5 text-sm text-gray-500 dark:text-gray-400">
        Choose who the assistant answers. It runs on the account you pair in Integrations.
      </p>

      <div
        role="radiogroup"
        aria-label="WhatsApp agent access"
        className="mb-4 flex gap-1 rounded-xl bg-gray-100 p-1 dark:bg-gray-900"
      >
        <button
          type="button"
          role="radio"
          aria-checked={mode === 'allowlist'}
          onClick={() => setMode('allowlist')}
          className={`${segBase} ${mode === 'allowlist' ? segActive : segIdle}`}
        >
          Allowlist only
        </button>
        <button
          type="button"
          role="radio"
          aria-checked={mode === 'all'}
          onClick={() => setMode('all')}
          className={`${segBase} ${mode === 'all' ? segActive : segIdle}`}
        >
          All numbers
        </button>
      </div>

      {allowAll ? (
        <p className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          The assistant answers <span className="font-medium">every</span> number that messages it.
          The allowlist below is ignored while this is on.
        </p>
      ) : (
        <p className="mb-3 text-sm text-gray-500 dark:text-gray-400">
          The assistant answers only these numbers and ignores everyone else. The{' '}
          <span className="font-medium">first</span> number also receives reminders and the daily
          recap.
        </p>
      )}

      <div className={`space-y-2 ${allowAll ? 'mt-4 opacity-60' : ''}`}>
        {numbers.map((n, i) => (
          <div key={i} className="flex items-center gap-2">
            <span className="w-6 shrink-0 text-right text-xs text-gray-400 dark:text-gray-500">
              {i + 1}.
            </span>
            <input
              value={n}
              onChange={(e) => setAt(i, e.target.value)}
              placeholder="e.g. 6285121503971"
              inputMode="tel"
              disabled={allowAll}
              className={inputClass}
            />
            {numbers.length > 1 && (
              <button
                type="button"
                onClick={() => removeAt(i)}
                disabled={allowAll}
                className="shrink-0 text-sm font-medium text-gray-400 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-500 dark:hover:text-red-400"
                aria-label="Remove number"
              >
                Remove
              </button>
            )}
          </div>
        ))}
      </div>
      <button
        type="button"
        onClick={add}
        disabled={allowAll}
        className="mt-2 text-sm font-medium text-indigo-600 hover:text-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-indigo-400 dark:hover:text-indigo-300"
      >
        + Add number
      </button>

      <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
        Enter the number with country code (e.g. 62… for Indonesia), or paste a full JID like
        6285121503971@s.whatsapp.net. No + or leading 0.
      </p>

      {msg && (
        <p
          className={`mt-4 text-sm ${msg.ok ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}
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
