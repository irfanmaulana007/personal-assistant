import { useState, useEffect } from 'react';
import {
  getIntegrations,
  setComposioKey,
  connectIntegration,
  disconnectIntegration,
} from '../api/client';
import type {
  Integrations as IntegrationsData,
  IntegrationToolkit,
  IntegrationStatus,
} from '../types';
import { WhatsAppCard } from './WhatsAppCard';

const inputClass =
  'rounded-xl border border-gray-200 dark:border-gray-700 dark:bg-gray-900 px-3 py-2.5 text-sm text-gray-900 dark:text-gray-100 outline-none transition focus:border-indigo-500 dark:focus:border-indigo-400 focus:ring-2 focus:ring-indigo-200 dark:focus:ring-indigo-500/30';

const statusStyles: Record<IntegrationStatus, { label: string; cls: string }> = {
  connected: {
    label: 'Connected',
    cls: 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300',
  },
  pending: {
    label: 'Pending',
    cls: 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  },
  error: { label: 'Error', cls: 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300' },
  disconnected: {
    label: 'Not connected',
    cls: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
  },
};

export function Integrations() {
  const [data, setData] = useState<IntegrationsData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);

  const reload = () => setRefreshKey((k) => k + 1);

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
        if (active) setError(e instanceof Error ? e.message : 'Failed to load integrations');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [refreshKey]);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Integrations
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Connect your apps through Composio.
          </p>
        </div>
        <button
          onClick={reload}
          className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 transition hover:bg-gray-50 dark:hover:bg-gray-800/60"
        >
          Refresh
        </button>
      </div>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">Loading…</p>
      ) : data ? (
        <>
          <ComposioKeyCard data={data} onSaved={setData} />
          {data.configured && (
            <div className="mt-6 grid gap-4 sm:grid-cols-2">
              {data.toolkits.map((t) => (
                <ToolkitCard key={t.slug} toolkit={t} onChanged={setData} />
              ))}
            </div>
          )}
          <WhatsAppCard />
        </>
      ) : null}
    </div>
  );
}

function ComposioKeyCard({
  data,
  onSaved,
}: {
  data: IntegrationsData;
  onSaved: (d: IntegrationsData) => void;
}) {
  const [apiKey, setApiKey] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMsg('');
    try {
      onSaved(await setComposioKey(apiKey.trim()));
      setApiKey('');
      setMsg('Saved');
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form
      onSubmit={save}
      className="mt-6 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5"
    >
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-50">Composio API key</h2>
        {data.configured ? (
          <span className="rounded-full bg-green-100 dark:bg-green-500/15 px-3 py-1 text-xs font-medium text-green-700 dark:text-green-300">
            Configured
          </span>
        ) : (
          <span className="rounded-full bg-amber-100 dark:bg-amber-500/15 px-3 py-1 text-xs font-medium text-amber-700 dark:text-amber-300">
            Not configured
          </span>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <input
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={
            data.configured
              ? `Saved (${data.api_key_mask}) — leave blank to keep`
              : 'Paste your Composio API key'
          }
          autoComplete="off"
          className={`${inputClass} flex-1 min-w-[240px]`}
        />
        <button
          type="submit"
          disabled={busy}
          className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
        >
          Save
        </button>
        {msg && <span className="text-sm text-gray-500 dark:text-gray-400">{msg}</span>}
      </div>
      <p className="mt-2 text-xs text-gray-400 dark:text-gray-500">
        Stored encrypted on the server. Get a key from the Composio dashboard.
      </p>
    </form>
  );
}

function ToolkitCard({
  toolkit,
  onChanged,
}: {
  toolkit: IntegrationToolkit;
  onChanged: (d: IntegrationsData) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const status = statusStyles[toolkit.status];
  const isConnected = toolkit.status === 'connected';

  const connect = async () => {
    setBusy(true);
    setErr('');
    try {
      const { redirect_url } = await connectIntegration(toolkit.slug);
      window.open(redirect_url, '_blank', 'noopener,noreferrer');
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not start connection');
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async (connectionId?: string) => {
    setBusy(true);
    setErr('');
    try {
      onChanged(await disconnectIntegration(toolkit.slug, connectionId));
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not disconnect');
    } finally {
      setBusy(false);
    }
  };

  const accounts = toolkit.accounts ?? [];

  return (
    <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-100 dark:bg-indigo-500/15 text-sm font-semibold text-indigo-700 dark:text-indigo-300">
            {toolkit.name.charAt(0)}
          </div>
          <div>
            <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
              {toolkit.name}
            </div>
            <span
              className={`mt-0.5 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${status.cls}`}
            >
              {status.label}
            </span>
          </div>
        </div>

        {/* Multi-account toolkits (e.g. Google Calendar): add another account.
            Single toolkits: the usual Connect / Disconnect. */}
        {toolkit.multi ? (
          <button
            onClick={connect}
            disabled={busy}
            className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
          >
            {busy ? 'Opening…' : accounts.length > 0 ? 'Add account' : 'Connect'}
          </button>
        ) : isConnected ? (
          <button
            onClick={() => disconnect()}
            disabled={busy}
            className="rounded-xl border border-gray-200 dark:border-gray-700 px-3 py-2 text-sm font-medium text-red-600 dark:text-red-400 transition hover:bg-red-50 dark:hover:bg-red-500/15 disabled:opacity-50"
          >
            Disconnect
          </button>
        ) : (
          <button
            onClick={connect}
            disabled={busy}
            className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
          >
            {busy ? 'Opening…' : 'Connect'}
          </button>
        )}
      </div>

      {toolkit.multi && accounts.length > 0 && (
        <div className="mt-4 divide-y divide-gray-100 dark:divide-gray-800 rounded-xl border border-gray-100 dark:border-gray-800">
          {accounts.map((a, i) => (
            <div
              key={a.connection_id}
              className="flex items-center justify-between gap-3 px-3 py-2"
            >
              <div className="min-w-0">
                <div className="truncate text-sm text-gray-700 dark:text-gray-200">
                  {a.label || `Account ${i + 1}`}
                </div>
                <span
                  className={`mt-0.5 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusStyles[a.status].cls}`}
                >
                  {statusStyles[a.status].label}
                </span>
              </div>
              <button
                onClick={() => disconnect(a.connection_id)}
                disabled={busy}
                className="shrink-0 rounded-lg border border-gray-200 dark:border-gray-700 px-2.5 py-1.5 text-xs font-medium text-red-600 dark:text-red-400 transition hover:bg-red-50 dark:hover:bg-red-500/15 disabled:opacity-50"
              >
                Disconnect
              </button>
            </div>
          ))}
        </div>
      )}

      {toolkit.multi && accounts.length === 0 && (
        <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
          Connect one or more Google accounts — events from all of them show up in your schedule.
        </p>
      )}

      {toolkit.status === 'pending' && (
        <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
          Authorization started — finish in the opened tab, then click Refresh.
        </p>
      )}
      {err && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{err}</p>}
    </div>
  );
}
