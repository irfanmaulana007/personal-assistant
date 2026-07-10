import { useState, useEffect } from 'react';
import { getPricing, setPricing, deletePricing } from '../../api/client';
import type { ModelPrice } from '../../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

export function PricingSettings() {
  const [prices, setPrices] = useState<ModelPrice[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [form, setForm] = useState({ model: '', input: '', output: '' });
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState('');

  useEffect(() => {
    let active = true;
    getPricing()
      .then((p) => active && setPrices(p))
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load pricing'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    const model = form.model.trim();
    if (!model) return;
    setBusy(true);
    setError('');
    setSaved('');
    try {
      setPrices(await setPricing(model, Number(form.input) || 0, Number(form.output) || 0));
      setForm({ model: '', input: '', output: '' });
      setSaved(`Saved rate for ${model}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setBusy(false);
    }
  };

  const reset = async (model: string) => {
    setBusy(true);
    setError('');
    try {
      setPrices(await deletePricing(model));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset');
    } finally {
      setBusy(false);
    }
  };

  const edit = (p: ModelPrice) =>
    setForm({ model: p.model, input: String(p.input_per_1m), output: String(p.output_per_1m) });

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
      <h2 className="mb-1 text-base font-semibold text-gray-900 dark:text-gray-50">
        Model pricing
      </h2>
      <p className="mb-5 text-sm text-gray-500 dark:text-gray-400">
        LLM responses report token usage, not cost — so cost is estimated from a per-model rate. Add
        or override a model's rate (USD per 1M tokens) to fix any run showing $0.
      </p>

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading…</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400 dark:border-gray-800 dark:text-gray-500">
                <th className="pb-2 font-medium">Model</th>
                <th className="pb-2 text-right font-medium">Input / 1M</th>
                <th className="pb-2 text-right font-medium">Output / 1M</th>
                <th className="pb-2 text-right font-medium">Source</th>
                <th className="pb-2 text-right font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {prices.map((p) => (
                <tr
                  key={p.model}
                  className="border-b border-gray-50 last:border-0 dark:border-gray-800"
                >
                  <td className="py-2.5 text-gray-800 dark:text-gray-100">{p.model}</td>
                  <td className="py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                    ${p.input_per_1m}
                  </td>
                  <td className="py-2.5 text-right tabular-nums text-gray-600 dark:text-gray-300">
                    ${p.output_per_1m}
                  </td>
                  <td className="py-2.5 text-right">
                    <span
                      className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                        p.source === 'custom'
                          ? 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
                          : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'
                      }`}
                    >
                      {p.source}
                    </span>
                  </td>
                  <td className="py-2.5 text-right">
                    <button
                      onClick={() => edit(p)}
                      className="rounded-lg px-2 py-1 text-xs font-medium text-gray-600 transition hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
                    >
                      Edit
                    </button>
                    {p.source === 'custom' && (
                      <button
                        onClick={() => reset(p.model)}
                        disabled={busy}
                        className="rounded-lg px-2 py-1 text-xs font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-500/15"
                      >
                        Reset
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <form onSubmit={save} className="mt-5 border-t border-gray-100 pt-4 dark:border-gray-800">
        <div className="text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
          Add or override a model
        </div>
        <div className="mt-2 flex flex-wrap items-end gap-2">
          <div className="min-w-[180px] flex-1">
            <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">Model</label>
            <input
              value={form.model}
              onChange={(e) => setForm({ ...form, model: e.target.value })}
              placeholder="deepseek-v4-flash"
              className={inputClass}
            />
          </div>
          <div className="w-32">
            <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">
              Input $/1M
            </label>
            <input
              type="number"
              min={0}
              step="0.01"
              value={form.input}
              onChange={(e) => setForm({ ...form, input: e.target.value })}
              placeholder="0.10"
              className={inputClass}
            />
          </div>
          <div className="w-32">
            <label className="mb-1 block text-xs text-gray-500 dark:text-gray-400">
              Output $/1M
            </label>
            <input
              type="number"
              min={0}
              step="0.01"
              value={form.output}
              onChange={(e) => setForm({ ...form, output: e.target.value })}
              placeholder="0.30"
              className={inputClass}
            />
          </div>
          <button
            type="submit"
            disabled={busy || !form.model.trim()}
            className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
          >
            Save
          </button>
        </div>
      </form>

      {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {saved && <p className="mt-3 text-sm text-green-600 dark:text-green-400">{saved}</p>}
      <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
        Rates are USD per 1,000,000 tokens. Cost is then shown in your selected currency (Settings →
        Display).
      </p>
    </div>
  );
}
