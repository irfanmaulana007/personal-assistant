import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { getIntegrations, setTrelloCreds } from '../api/client';
import { useProjects } from '../contexts/project';
import type { Integrations as IntegrationsData } from '../types';
import { SkeletonFormCard } from './ui/Skeleton';
import { useIsDark } from '../lib/useChartTheme';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

// Trello brand mark (Simple Icons), tinted per theme so it stays legible on
// the dark card background.
const TRELLO_ICON_PATH =
  'M21 0H3C1.343 0 0 1.343 0 3v18c0 1.656 1.343 3 3 3h18c1.656 0 3-1.344 3-3V3c0-1.657-1.344-3-3-3zM10.44 18.18c0 .795-.645 1.44-1.44 1.44H4.56c-.795 0-1.44-.646-1.44-1.44V4.56c0-.795.645-1.44 1.44-1.44H9c.795 0 1.44.645 1.44 1.44v13.62zm10.44-6c0 .794-.645 1.44-1.44 1.44H15c-.795 0-1.44-.646-1.44-1.44V4.56c0-.795.646-1.44 1.44-1.44h4.44c.795 0 1.44.645 1.44 1.44v7.62z';

function TrelloIcon() {
  const dark = useIsDark();
  return (
    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gray-50 ring-1 ring-inset ring-gray-100 dark:bg-gray-700 dark:ring-gray-600">
      <svg
        role="img"
        aria-label="Trello"
        viewBox="0 0 24 24"
        className="h-5 w-5"
        fill={dark ? '#579DFF' : '#0079BF'}
      >
        <path d={TRELLO_ICON_PATH} />
      </svg>
    </div>
  );
}

// TrelloCredsCard collects the two Trello secrets (API key + user token) that
// authenticate every Trello request. Both must be filled to Save; Clear wipes
// both. A blank Save (both empty) is a no-op so it doesn't clobber stored creds.
function TrelloCredsCard({
  configured,
  keyMask,
  tokenMask,
  onSave,
}: {
  configured: boolean;
  keyMask: string;
  tokenMask: string;
  onSave: (apiKey: string, token: string) => Promise<IntegrationsData>;
}) {
  const [apiKey, setApiKey] = useState('');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const k = apiKey.trim();
    const t = token.trim();
    if (k === '' && t === '') return; // blank Save keeps existing creds
    if (k === '' || t === '') {
      setMsg('Enter both the API key and the token');
      return;
    }
    setBusy(true);
    setMsg('');
    try {
      await onSave(k, t);
      setApiKey('');
      setToken('');
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
      await onSave('', '');
      setApiKey('');
      setToken('');
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
        <div className="flex items-center gap-3">
          <TrelloIcon />
          <h2 className="text-base font-semibold text-gray-900 dark:text-gray-50">Credentials</h2>
        </div>
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
      <div className="space-y-3">
        <input
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={
            configured
              ? `API key saved (${keyMask}) — leave blank to keep`
              : 'Paste your Trello API key'
          }
          autoComplete="off"
          className={inputClass}
        />
        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={
            configured
              ? `Token saved (${tokenMask}) — leave blank to keep`
              : 'Paste your Trello token'
          }
          autoComplete="off"
          className={inputClass}
        />
      </div>
      <p className="mt-2 text-xs text-gray-400 dark:text-gray-500">
        Stored encrypted on the server. Powers the{' '}
        <span className="font-medium">Trello Board Review</span> and{' '}
        <span className="font-medium">Trello Card Creator</span> skills — enable them under Skills.
        Get your API key and token from trello.com/app-key.
      </p>
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
            Clear
          </button>
        )}
        {msg && <span className="text-sm text-gray-500 dark:text-gray-400">{msg}</span>}
      </div>
    </form>
  );
}

// Trello integration detail page. Mirrors the WhatsApp integration detail page:
// a back-link to the integrations list, a header, then the credentials card
// that used to live under Settings → API keys.
export function IntegrationsTrello() {
  const { projectPath } = useProjects();
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
        if (active) setError(e instanceof Error ? e.message : 'Failed to load Trello integration');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <Link
        to={projectPath('integrations')}
        className="inline-flex items-center gap-1 text-sm font-medium text-gray-500 transition hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
      >
        <svg viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4" aria-hidden="true">
          <path
            fillRule="evenodd"
            d="M12.79 5.23a.75.75 0 0 1-.02 1.06L8.832 10l3.938 3.71a.75.75 0 1 1-1.04 1.08l-4.5-4.25a.75.75 0 0 1 0-1.08l4.5-4.25a.75.75 0 0 1 1.06.02z"
            clipRule="evenodd"
          />
        </svg>
        Integrations
      </Link>

      <div className="mt-2">
        <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
          Trello
        </h1>
        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
          Connect Trello so the assistant can review boards and create cards.
        </p>
      </div>

      <div className="mt-6 space-y-6">
        {loading ? (
          <SkeletonFormCard fields={2} />
        ) : error ? (
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        ) : data ? (
          <TrelloCredsCard
            configured={data.trello_configured}
            keyMask={data.trello_key_mask}
            tokenMask={data.trello_token_mask}
            onSave={async (apiKey, token) => {
              const d = await setTrelloCreds(apiKey, token);
              setData(d);
              return d;
            }}
          />
        ) : null}
      </div>
    </div>
  );
}
