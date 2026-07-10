import { useState, useEffect } from 'react';
import { getWhatsAppAllowlist, setWhatsAppAllowlist } from '../../api/client';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

// Show a JID as the friendlier bare number in the input.
function toDisplay(jid: string): string {
  return jid.includes('@') ? jid.split('@')[0] : jid;
}

export function WhatsAppSettings() {
  const [numbers, setNumbers] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    let active = true;
    getWhatsAppAllowlist()
      .then((d) => {
        if (!active) return;
        setNumbers(d.allowlist.length ? d.allowlist.map(toDisplay) : ['']);
      })
      .catch(() => {})
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

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
      const d = await setWhatsAppAllowlist(cleaned);
      setNumbers(d.allowlist.length ? d.allowlist.map(toDisplay) : ['']);
      setMsg({ ok: true, text: 'Allowed numbers saved.' });
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to save' });
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p className="text-sm text-gray-500">Loading…</p>;

  return (
    <form onSubmit={save} className="rounded-2xl border border-gray-200 bg-white p-6">
      <h2 className="mb-1 text-base font-semibold text-gray-900">WhatsApp numbers</h2>
      <p className="mb-5 text-sm text-gray-500">
        The numbers allowed to chat with your assistant. The assistant runs on the account you pair
        in Integrations; it answers these numbers and ignores everyone else. The{' '}
        <span className="font-medium">first</span> number also receives reminders and the daily
        recap.
      </p>

      <div className="space-y-2">
        {numbers.map((n, i) => (
          <div key={i} className="flex items-center gap-2">
            <span className="w-6 shrink-0 text-right text-xs text-gray-400">{i + 1}.</span>
            <input
              value={n}
              onChange={(e) => setAt(i, e.target.value)}
              placeholder="e.g. 6285121503971"
              inputMode="tel"
              className={inputClass}
            />
            {numbers.length > 1 && (
              <button
                type="button"
                onClick={() => removeAt(i)}
                className="shrink-0 text-sm font-medium text-gray-400 hover:text-red-600"
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
        className="mt-2 text-sm font-medium text-indigo-600 hover:text-indigo-700"
      >
        + Add number
      </button>

      <p className="mt-3 text-xs text-gray-400">
        Enter the number with country code (e.g. 62… for Indonesia), or paste a full JID like
        6285121503971@s.whatsapp.net. No + or leading 0.
      </p>

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
