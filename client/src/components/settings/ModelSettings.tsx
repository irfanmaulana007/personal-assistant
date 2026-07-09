import { useState } from 'react';
import { useSettings } from '../../hooks/useSettings';
import type { LlmTestResult } from '../../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

export function ModelSettings() {
  const { settings, loading, error, save, test } = useSettings();

  const [apiKey, setApiKey] = useState('');
  const [providerInput, setProviderInput] = useState<string | null>(null);
  const [modelInput, setModelInput] = useState<string | null>(null);
  const [baseURLInput, setBaseURLInput] = useState<string | null>(null);
  const provider = providerInput ?? settings?.provider ?? 'deepseek';
  const model = modelInput ?? settings?.model ?? '';
  const baseURL = baseURLInput ?? settings?.base_url ?? '';
  const providers = settings?.providers ?? [];

  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<LlmTestResult | null>(null);

  const handleProviderChange = (id: string) => {
    setProviderInput(id);
    const preset = providers.find((p) => p.id === id);
    if (preset) {
      setModelInput(preset.default_model);
      setBaseURLInput(preset.default_base_url);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setSaved(false);
    setTestResult(null);
    const update: { provider: string; model: string; base_url: string; api_key?: string } = {
      provider,
      model: model.trim(),
      base_url: baseURL.trim(),
    };
    if (apiKey !== '') update.api_key = apiKey.trim();
    const ok = await save(update);
    if (ok) {
      setApiKey('');
      setProviderInput(null);
      setModelInput(null);
      setBaseURLInput(null);
      setSaved(true);
    }
    setSaving(false);
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    setTestResult(await test());
    setTesting(false);
  };

  const handleClearKey = async () => {
    setTestResult(null);
    setSaved(false);
    await save({ api_key: '' });
  };

  if (loading) return <p className="text-sm text-gray-500">Loading…</p>;

  return (
    <form onSubmit={handleSave} className="rounded-2xl border border-gray-200 bg-white p-6">
      <div className="mb-5 flex items-center justify-between">
        <h2 className="text-base font-semibold text-gray-900">LLM Provider</h2>
        {settings?.configured ? (
          <span className="rounded-full bg-green-100 px-3 py-1 text-xs font-medium text-green-700">
            Configured
          </span>
        ) : (
          <span className="rounded-full bg-amber-100 px-3 py-1 text-xs font-medium text-amber-700">
            Not configured
          </span>
        )}
      </div>

      <div className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Provider</label>
          <select
            value={provider}
            onChange={(e) => handleProviderChange(e.target.value)}
            className={inputClass}
          >
            {providers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </select>
          <p className="mt-1 text-xs text-gray-400">
            All providers use an OpenAI-compatible API. Choosing one fills in its default endpoint
            and model.
          </p>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">API Key</label>
          <input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder={
              settings?.configured
                ? `Saved (${settings.api_key_mask}) — leave blank to keep`
                : `Paste your ${providers.find((p) => p.id === provider)?.label ?? ''} API key`
            }
            className={inputClass}
            autoComplete="off"
          />
          <p className="mt-1 text-xs text-gray-400">
            Stored encrypted on the server. Leave blank to keep the existing key.
          </p>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Model</label>
          <input
            type="text"
            value={model}
            onChange={(e) => setModelInput(e.target.value)}
            placeholder="deepseek-chat"
            className={inputClass}
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Base URL</label>
          <input
            type="text"
            value={baseURL}
            onChange={(e) => setBaseURLInput(e.target.value)}
            placeholder="https://api.deepseek.com"
            className={inputClass}
          />
          <p className="mt-1 text-xs text-gray-400">
            Any OpenAI-compatible endpoint works (DeepSeek, OpenAI, …).
          </p>
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

      {testResult && (
        <p className={`mt-4 text-sm ${testResult.ok ? 'text-green-600' : 'text-red-600'}`}>
          {testResult.ok
            ? `Connection OK${testResult.model ? ` (${testResult.model})` : ''}`
            : `Connection failed: ${testResult.error}`}
        </p>
      )}

      <div className="mt-6 flex flex-wrap items-center gap-3">
        <button
          type="submit"
          disabled={saving}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={handleTest}
          disabled={testing || !settings?.configured}
          className="rounded-xl border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {testing ? 'Testing…' : 'Test connection'}
        </button>
        {settings?.configured && (
          <button
            type="button"
            onClick={handleClearKey}
            className="rounded-xl px-4 py-2.5 text-sm font-medium text-red-600 transition hover:bg-red-50"
          >
            Clear key
          </button>
        )}
        {saved && <span className="text-sm text-green-600">Saved</span>}
      </div>
    </form>
  );
}
