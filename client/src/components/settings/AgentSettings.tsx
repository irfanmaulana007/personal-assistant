import { useState, useEffect } from 'react';
import { getPersona, updatePersona } from '../../api/client';
import type { Persona } from '../../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

const OPTIONS: Record<string, { value: string; label: string }[]> = {
  tone: [
    { value: 'formal', label: 'Formal' },
    { value: 'balanced', label: 'Balanced' },
    { value: 'casual', label: 'Casual' },
  ],
  emoji: [
    { value: 'none', label: 'None' },
    { value: 'occasional', label: 'Occasional' },
    { value: 'frequent', label: 'Frequent' },
  ],
  length: [
    { value: 'concise', label: 'Concise' },
    { value: 'balanced', label: 'Balanced' },
    { value: 'detailed', label: 'Detailed' },
  ],
  personality: [
    { value: 'balanced', label: 'Balanced' },
    { value: 'professional', label: 'Professional' },
    { value: 'friendly', label: 'Friendly' },
    { value: 'witty', label: 'Witty' },
    { value: 'direct', label: 'Direct' },
    { value: 'encouraging', label: 'Encouraging' },
  ],
};

const DEFAULT: Persona = {
  tone: 'balanced',
  emoji: 'occasional',
  length: 'balanced',
  personality: 'balanced',
  name: '',
  custom: '',
};

export function AgentSettings() {
  const [persona, setPersona] = useState<Persona>(DEFAULT);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    let active = true;
    getPersona()
      .then((p) => active && setPersona(p))
      .catch(() => {})
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const set = (k: keyof Persona, v: string) => setPersona((p) => ({ ...p, [k]: v }));

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setMsg(null);
    try {
      setPersona(await updatePersona(persona));
      setMsg({ ok: true, text: 'Agent preferences saved.' });
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to save' });
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p className="text-sm text-gray-500">Loading…</p>;

  const fields: { k: keyof Persona; label: string; hint: string }[] = [
    {
      k: 'tone',
      label: 'Tone',
      hint: 'How formal the assistant sounds. Formal is polite and professional (uses “Anda” in Indonesian); Casual is relaxed and friendly (“kamu”).',
    },
    {
      k: 'emoji',
      label: 'Emoji usage',
      hint: 'How often the assistant adds emoji to its replies — none, occasionally, or a lot.',
    },
    {
      k: 'length',
      label: 'Response length',
      hint: 'How much detail you get. Concise keeps answers short and to the point; Detailed gives thorough explanations.',
    },
    {
      k: 'personality',
      label: 'Personality',
      hint: 'The assistant’s character — e.g. professional and neutral, friendly, witty, direct (no filler), or warm and encouraging.',
    },
  ];

  return (
    <form onSubmit={save} className="rounded-2xl border border-gray-200 bg-white p-6">
      <h2 className="mb-1 text-base font-semibold text-gray-900">Agent preference</h2>
      <p className="mb-5 text-sm text-gray-500">
        Shape how the assistant talks to you. These apply to your account only.
      </p>

      <div className="space-y-4">
        {fields.map((f) => (
          <div key={f.k}>
            <label className="mb-1 block text-sm font-medium text-gray-700">{f.label}</label>
            <select
              value={persona[f.k]}
              onChange={(e) => set(f.k, e.target.value)}
              className={inputClass}
            >
              {OPTIONS[f.k].map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <p className="mt-1 text-xs text-gray-400">{f.hint}</p>
          </div>
        ))}

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Assistant name</label>
          <input
            value={persona.name}
            onChange={(e) => set('name', e.target.value)}
            placeholder="e.g. My Assistant"
            className={inputClass}
          />
          <p className="mt-1 text-xs text-gray-400">
            What the assistant calls itself when it refers to itself. Leave blank to keep the
            default.
          </p>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Custom instructions
          </label>
          <textarea
            value={persona.custom}
            onChange={(e) => set('custom', e.target.value)}
            rows={3}
            placeholder="e.g. Always answer in bullet points; call me by my first name…"
            className={`${inputClass} resize-none`}
          />
          <p className="mt-1 text-xs text-gray-400">
            Any other guidance, in your own words, for how the assistant should behave.
          </p>
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
