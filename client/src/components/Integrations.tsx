import { useState, useEffect } from 'react';
import {
  getIntegrations,
  setComposioKey,
  setWebSearchKey,
  setOpenAIKey,
  connectIntegration,
  disconnectIntegration,
} from '../api/client';
import type {
  Integrations as IntegrationsData,
  IntegrationToolkit,
  IntegrationStatus,
} from '../types';
import { WhatsAppCard } from './WhatsAppCard';
import { Skeleton, SkeletonCard } from './ui/Skeleton';
import { useIsDark } from '../lib/useChartTheme';

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

// Brand marks for each supported toolkit, keyed by Composio slug. Single-path
// SVGs (from Simple Icons) drawn in the tool's brand color, with a generic
// fallback for any slug we don't have a mark for.
const toolkitIcons: Record<string, { color: string; darkColor?: string; path: string }> = {
  gmail: {
    color: '#EA4335',
    path: 'M24 5.457v13.909c0 .904-.732 1.636-1.636 1.636h-3.819V11.73L12 16.64l-6.545-4.91v9.273H1.636A1.636 1.636 0 0 1 0 19.366V5.457c0-2.023 2.309-3.178 3.927-1.964L5.455 4.64 12 9.548l6.545-4.91 1.528-1.145C21.69 2.28 24 3.434 24 5.457z',
  },
  googlecalendar: {
    color: '#4285F4',
    path: 'M18.316 5.684H24v12.632h-5.684V5.684zM5.684 24h12.632v-5.684H5.684V24zM18.316 5.684V0H1.895A1.894 1.894 0 0 0 0 1.895v16.421h5.684V5.684h12.632zm-7.207 6.25v-.065c.272-.144.5-.349.687-.617s.279-.595.279-.982c0-.379-.099-.72-.3-1.025a2.05 2.05 0 0 0-.832-.714 2.703 2.703 0 0 0-1.197-.257c-.6 0-1.094.156-1.481.467-.386.311-.65.671-.793 1.078l1.085.452c.086-.249.224-.461.413-.633.189-.172.445-.257.767-.257.33 0 .602.088.816.264a.86.86 0 0 1 .322.703c0 .33-.12.589-.36.778-.24.19-.535.284-.886.284h-.567v1.085h.633c.407 0 .748.109 1.02.327.272.218.407.499.407.843 0 .336-.129.614-.387.832s-.565.327-.924.327c-.351 0-.651-.103-.897-.311-.248-.208-.422-.502-.521-.881l-1.096.452c.178.616.505 1.082.977 1.401.472.319.984.478 1.538.477a2.84 2.84 0 0 0 1.293-.291c.382-.193.684-.458.902-.794.218-.336.327-.72.327-1.149 0-.429-.115-.797-.344-1.105a2.067 2.067 0 0 0-.881-.689zm2.093-1.931l.602.913L15 10.045v5.744h1.187V8.446h-.827l-2.158 1.557zM22.105 0h-3.289v5.184H24V1.895A1.894 1.894 0 0 0 22.105 0zm-3.289 23.5l4.684-4.684h-4.684V23.5zM0 22.105C0 23.152.848 24 1.895 24h3.289v-5.184H0v3.289z',
  },
  github: {
    color: '#181717',
    darkColor: '#f0f6fc',
    path: 'M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12',
  },
  sentry: {
    color: '#362D59',
    darkColor: '#9e86d6',
    path: 'M13.91 2.505c-.873-1.448-2.972-1.448-3.844 0L6.904 7.92a15.478 15.478 0 0 1 8.53 12.811h-2.221A13.301 13.301 0 0 0 5.784 9.814l-2.926 5.06a7.65 7.65 0 0 1 4.435 5.848H2.194a.365.365 0 0 1-.298-.534l1.413-2.402a5.16 5.16 0 0 0-1.614-.913L.296 19.275a2.182 2.182 0 0 0 .812 2.999 2.24 2.24 0 0 0 1.086.288h6.983a9.322 9.322 0 0 0-3.845-8.318l1.11-1.922a11.47 11.47 0 0 1 4.95 10.24h5.915a17.242 17.242 0 0 0-7.885-15.28l2.244-3.845a.37.37 0 0 1 .504-.13c.255.14 9.75 16.708 9.928 16.9a.365.365 0 0 1-.327.543h-2.287c.029.612.029 1.223 0 1.831h2.297a2.206 2.206 0 0 0 1.922-3.31z',
  },
};

// Generic "app" fallback mark for any toolkit without a brand icon.
const fallbackToolkitIcon = 'M4 4h6v6H4V4zm10 0h6v6h-6V4zM4 14h6v6H4v-6zm10 0h6v6h-6v-6z';

function ToolkitIcon({ slug, name }: { slug: string; name: string }) {
  const icon = toolkitIcons[slug];
  const dark = useIsDark();
  const fill = icon ? (dark && icon.darkColor ? icon.darkColor : icon.color) : '#6B7280';
  return (
    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gray-50 ring-1 ring-inset ring-gray-100 dark:bg-gray-700 dark:ring-gray-600">
      <svg role="img" aria-label={name} viewBox="0 0 24 24" className="h-5 w-5" fill={fill}>
        <path d={icon ? icon.path : fallbackToolkitIcon} />
      </svg>
    </div>
  );
}

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
        <>
          <SkeletonCard className="mt-6">
            <div className="mb-3 flex items-center justify-between">
              <Skeleton className="h-3.5 w-32" />
              <Skeleton className="h-5 w-20 rounded-full" />
            </div>
            <Skeleton className="h-10 w-full rounded-xl" />
          </SkeletonCard>
          <div className="mt-6 grid gap-4 sm:grid-cols-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <SkeletonCard key={i}>
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <Skeleton className="h-10 w-10 rounded-xl" />
                    <div>
                      <Skeleton className="h-3.5 w-24" />
                      <Skeleton className="mt-2 h-4 w-16 rounded-full" />
                    </div>
                  </div>
                  <Skeleton className="h-9 w-24 rounded-xl" />
                </div>
              </SkeletonCard>
            ))}
          </div>
        </>
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
          <WebSearchKeyCard data={data} onSaved={setData} />
          <OpenAIKeyCard data={data} onSaved={setData} />
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

function WebSearchKeyCard({
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
      onSaved(await setWebSearchKey(apiKey.trim()));
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
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-50">
          Web Search API key
        </h2>
        {data.web_search_configured ? (
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
            data.web_search_configured
              ? `Saved (${data.web_search_key_mask}) — leave blank to keep`
              : 'Paste your Tavily API key'
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
        Stored encrypted on the server. Powers the <span className="font-medium">Web Search</span>{' '}
        skill — enable it under Skills. Get a free key from the Tavily dashboard (1,000
        searches/month, no card required).
      </p>
    </form>
  );
}

function OpenAIKeyCard({
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
      onSaved(await setOpenAIKey(apiKey.trim()));
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
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-50">OpenAI API key</h2>
        {data.openai_configured ? (
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
            data.openai_configured
              ? `Saved (${data.openai_key_mask}) — leave blank to keep`
              : 'Paste your OpenAI API key'
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
        Stored encrypted on the server. Powers the{' '}
        <span className="font-medium">Image Generator</span> skill (OpenAI gpt-image-1) — enable it
        under Skills. Get a key from the OpenAI platform dashboard; image generation is billed
        per-image (roughly $0.01–0.04 each).
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
          <ToolkitIcon slug={toolkit.slug} name={toolkit.name} />
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
