import { useState, useEffect } from 'react';
import { getIntegrations, setWebSearchKey, setOpenAIKey } from '../../api/client';
import type { Integrations as IntegrationsData } from '../../types';
import { SkeletonFormCard } from '../ui/Skeleton';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

// A single project-wide API key: title, configured pill, password input, Save,
// and a Clear button once a key is stored. Saving an empty field is a no-op
// (the placeholder says "leave blank to keep"); use Clear to remove the key.
function KeyCard({
  title,
  configured,
  mask,
  placeholder,
  onSave,
  children,
}: {
  title: string;
  configured: boolean;
  mask: string;
  placeholder: string;
  onSave: (key: string) => Promise<IntegrationsData>;
  children: React.ReactNode;
}) {
  const [apiKey, setApiKey] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = apiKey.trim();
    if (trimmed === '') return; // blank Save keeps the existing key
    setBusy(true);
    setMsg('');
    try {
      await onSave(trimmed);
      setApiKey('');
      setMsg('Saved');
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setBusy(false);
    }
  };

  const clear = async () => {
    setBusy(true);
    setMsg('');
    try {
      await onSave('');
      setApiKey('');
      setMsg('Cleared');
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Failed to clear');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form
      onSubmit={submit}
      className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800"
    >
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-50">{title}</h2>
        {configured ? (
          <span className="rounded-full bg-green-100 px-3 py-1 text-xs font-medium text-green-700 dark:bg-green-500/15 dark:text-green-300">
            Configured
          </span>
        ) : (
          <span className="rounded-full bg-amber-100 px-3 py-1 text-xs font-medium text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
            Not configured
          </span>
        )}
      </div>
      <input
        type="password"
        value={apiKey}
        onChange={(e) => setApiKey(e.target.value)}
        placeholder={configured ? `Saved (${mask}) — leave blank to keep` : placeholder}
        autoComplete="off"
        className={inputClass}
      />
      <p className="mt-2 text-xs text-gray-400 dark:text-gray-500">{children}</p>
      <div className="mt-5 flex flex-wrap items-center gap-3">
        <button
          type="submit"
          disabled={busy}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
        {configured && (
          <button
            type="button"
            onClick={clear}
            disabled={busy}
            className="rounded-xl px-4 py-2.5 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-500/15"
          >
            Clear key
          </button>
        )}
        {msg && <span className="text-sm text-gray-500 dark:text-gray-400">{msg}</span>}
      </div>
    </form>
  );
}

// ApiKeysSection renders the project's skill API-key cards (Web Search, OpenAI).
// It lives inside the Model settings page as a sub-section, loading its own
// integration data independently of the LLM-provider card above it.
export function ApiKeysSection() {
  const [data, setData] = useState<IntegrationsData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    getIntegrations()
      .then((d) => {
        if (active) {
          setData(d);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load API keys');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  if (loading) {
    return (
      <div className="space-y-6">
        <SkeletonFormCard fields={1} />
        <SkeletonFormCard fields={1} />
      </div>
    );
  }

  if (error) return <p className="text-sm text-red-600 dark:text-red-400">{error}</p>;
  if (!data) return null;

  return (
    <div className="space-y-6">
      <KeyCard
        title="Web Search API key"
        configured={data.web_search_configured}
        mask={data.web_search_key_mask}
        placeholder="Paste your Tavily API key"
        onSave={async (key) => {
          const d = await setWebSearchKey(key);
          setData(d);
          return d;
        }}
      >
        Stored encrypted on the server. Powers the <span className="font-medium">Web Search</span>{' '}
        skill — enable it under Skills. Get a free key from the Tavily dashboard (1,000
        searches/month, no card required).
      </KeyCard>

      <KeyCard
        title="OpenAI API key"
        configured={data.openai_configured}
        mask={data.openai_key_mask}
        placeholder="Paste your OpenAI API key"
        onSave={async (key) => {
          const d = await setOpenAIKey(key);
          setData(d);
          return d;
        }}
      >
        Stored encrypted on the server. Powers the{' '}
        <span className="font-medium">Image Generator</span> skill (OpenAI gpt-image-1) — enable it
        under Skills. Get a key from the OpenAI platform dashboard; image generation is billed
        per-image (roughly $0.01–0.04 each).
      </KeyCard>
    </div>
  );
}
