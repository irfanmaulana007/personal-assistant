import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import {
  getMCPIntegrations,
  setMCPServer,
  testMCPServer,
  connectMCPOAuth,
  disconnectMCPOAuth,
  setNotionTarget,
  deleteNotionTarget,
} from '../api/client';
import { useProjects } from '../contexts/project';
import type { MCPIntegrations, MCPServer, MCPMode, NotionTarget } from '../types';
import { SkeletonFormCard } from './ui/Skeleton';
import { useIsDark } from '../lib/useChartTheme';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

const labelClass = 'block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1';

// Brand marks (Simple Icons) for each provider, tinted per theme.
const providerIcons: Record<string, { light: string; dark: string; path: string }> = {
  notion: {
    light: '#000000',
    dark: '#ffffff',
    path: 'M4.459 4.208c.746.606 1.026.56 2.428.466l13.215-.793c.28 0 .047-.28-.046-.326L17.86 1.968c-.42-.326-.981-.7-2.055-.607L3.01 2.295c-.466.046-.56.28-.374.466zm.793 3.08v13.904c0 .747.373 1.027 1.214.98l14.523-.84c.841-.046.935-.56.935-1.167V6.354c0-.606-.233-.933-.748-.887l-15.177.887c-.56.047-.747.327-.747.933zm14.337.745c.093.42 0 .84-.42.888l-.7.14v10.264c-.608.327-1.168.514-1.635.514-.748 0-.935-.234-1.495-.933l-4.577-7.186v6.952l1.448.328s0 .84-1.168.84l-3.222.186c-.093-.187 0-.653.327-.746l.84-.233V9.854L7.822 9.76c-.094-.42.14-1.026.793-1.073l3.456-.233 4.764 7.279v-6.44l-1.215-.139c-.093-.514.28-.887.747-.933zM1.936 1.035l13.31-.98c1.634-.14 2.055-.047 3.082.7l4.249 2.986c.7.513.934.653.934 1.213v16.378c0 1.026-.373 1.634-1.68 1.726l-15.458.934c-.98.047-1.448-.093-1.962-.747l-3.129-4.06c-.56-.747-.793-1.306-.793-1.96V2.667c0-.839.374-1.54 1.216-1.632z',
  },
  cloudflare: {
    light: '#F38020',
    dark: '#F38020',
    path: 'M16.5088 16.8447c.1475-.5068.0908-.9707-.1553-1.3154-.2246-.3164-.6045-.499-1.0615-.5205l-8.6592-.1123a.1559.1559 0 0 1-.127-.0713.1618.1618 0 0 1-.0195-.1483c.0233-.0693.0891-.1197.1607-.1263l8.7315-.1123c1.0351-.0479 2.1592-.8867 2.5527-1.9151l.499-1.3018c.0195-.0546.0273-.1123.0136-.168-.5654-2.5605-2.8496-4.4707-5.583-4.4707-2.5195 0-4.6592 1.626-5.4258 3.8848a2.789 2.789 0 0 0-1.958-.5449c-1.3652.1367-2.4609 1.2324-2.5977 2.5976a2.919 2.919 0 0 0 .0674.9553C.9941 13.373 0 14.4267 0 15.7207c0 .1152.0079.2304.0234.3437a.1372.1372 0 0 0 .1348.1181h15.9843c.0723 0 .1367-.0479.1573-.1181l.209-.7197zm2.7285-5.1445c-.0791 0-.1601.002-.2402.0058-.0557.002-.1055.0411-.127.0938l-.3418.9707c-.1476.5068-.0908.9707.1553 1.3154.2246.3164.6045.499 1.0615.5205l1.8467.1123a.1544.1544 0 0 1 .123.0723.1614.1614 0 0 1 .0195.1474c-.0234.0703-.0888.1201-.1601.1269l-1.9209.1123c-1.04.0479-2.1592.8867-2.5527 1.9151l-.1387.3603c-.0273.0703.0234.1406.0996.1406h6.6074c.0664 0 .125-.0429.1445-.1074.1153-.4131.1758-.8477.1758-1.2969 0-2.6923-2.1924-4.8847-4.8925-4.8847',
  },
  railway: {
    light: '#0B0D0E',
    dark: '#ffffff',
    path: 'M.113 10.27A12.328 12.328 0 0 0 0 11.501h17.928c.052 0 .13-.045.216-.12a10.831 10.831 0 0 0-.114-.24c-.03-.06-.06-.12-.09-.181-.06-.12-.12-.24-.18-.36-.03-.06-.06-.12-.09-.18a12.06 12.06 0 0 0-.21-.39l-.03-.06a11.99 11.99 0 0 0-.87-1.29l-.03-.03a11.99 11.99 0 0 0-2.7-2.58A11.94 11.94 0 0 0 6.75.75a11.94 11.94 0 0 0-4.5 3.75A11.94 11.94 0 0 0 .113 10.27zM24 12.75H6.072c-.052 0-.13.045-.216.12.038.08.076.16.114.24.03.06.06.12.09.181.06.12.12.24.18.36.03.06.06.12.09.18.072.13.142.26.21.39l.03.06c.27.44.562.87.87 1.29l.03.03c.81 1.02 1.74 1.89 2.7 2.58a11.94 11.94 0 0 0 6.99 2.25 11.94 11.94 0 0 0 4.5-3.75A11.94 11.94 0 0 0 24 12.75z',
  },
};

function ProviderIcon({ slug, name, dark }: { slug: string; name: string; dark: boolean }) {
  const icon = providerIcons[slug];
  return (
    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gray-50 ring-1 ring-inset ring-gray-100 dark:bg-gray-700 dark:ring-gray-600">
      {icon ? (
        <svg
          role="img"
          aria-label={name}
          viewBox="0 0 24 24"
          className="h-5 w-5"
          fill={dark ? icon.dark : icon.light}
        >
          <path d={icon.path} />
        </svg>
      ) : (
        <span className="text-sm font-semibold text-gray-500 dark:text-gray-300">{name[0]}</span>
      )}
    </div>
  );
}

function StatusBadge({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span
      className={`mt-0.5 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
        ok
          ? 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300'
          : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'
      }`}
    >
      {label}
    </span>
  );
}

// One MCP provider card. Enablement is the provider's skill (managed on the
// Skills page); this card manages credentials/connection + access mode.
function MCPServerCard({
  server,
  onChanged,
  skillsPath,
}: {
  server: MCPServer;
  onChanged: (d: MCPIntegrations) => void;
  skillsPath: string;
}) {
  const [mode, setMode] = useState<MCPMode>(server.mode);
  const [endpoint, setEndpoint] = useState(server.endpoint);
  const [token, setToken] = useState('');
  const [showEndpoint, setShowEndpoint] = useState(false);
  const [busy, setBusy] = useState(false);
  const [testing, setTesting] = useState(false);
  const [msg, setMsg] = useState('');
  const [testMsg, setTestMsg] = useState('');
  const dark = useIsDark();

  const isOAuth = server.auth === 'oauth';
  const ready = isOAuth ? !!server.connected : !!server.configured;

  const save = async () => {
    setBusy(true);
    setMsg('');
    try {
      const payload: { mode: string; endpoint: string; token?: string } = {
        mode,
        endpoint: endpoint.trim(),
      };
      if (!isOAuth && token.trim() !== '') payload.token = token.trim();
      onChanged(await setMCPServer(server.slug, payload));
      setToken('');
      setMsg('Saved');
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    setTesting(true);
    setTestMsg('');
    try {
      const res = await testMCPServer(
        server.slug,
        isOAuth ? undefined : { endpoint: endpoint.trim(), token: token.trim() || undefined },
      );
      setTestMsg(`✓ Connected — ${res.tool_count} tools available`);
    } catch (e) {
      setTestMsg(`✗ ${e instanceof Error ? e.message : 'Connection failed'}`);
    } finally {
      setTesting(false);
    }
  };

  const connect = async () => {
    setBusy(true);
    setMsg('');
    try {
      const { redirect_url } = await connectMCPOAuth(server.slug);
      window.open(redirect_url, '_blank', 'noopener,noreferrer');
      setMsg('Authorize in the new tab, then return here — the page refreshes automatically.');
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'Could not start the connection');
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    setBusy(true);
    setMsg('');
    try {
      onChanged(await disconnectMCPOAuth(server.slug));
      setMsg('Disconnected');
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'Could not disconnect');
    } finally {
      setBusy(false);
    }
  };

  const clearToken = async () => {
    setBusy(true);
    setMsg('');
    try {
      onChanged(await setMCPServer(server.slug, { mode, endpoint: endpoint.trim(), token: '' }));
      setToken('');
      setMsg('Token removed');
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'Failed to remove token');
    } finally {
      setBusy(false);
    }
  };

  const statusLabel = ready
    ? mode === 'readwrite'
      ? `${isOAuth ? 'Connected' : 'Configured'} · read & write`
      : `${isOAuth ? 'Connected' : 'Configured'} · read-only`
    : isOAuth
      ? 'Not connected'
      : 'Not configured';

  return (
    <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <ProviderIcon slug={server.slug} name={server.name} dark={dark} />
          <div>
            <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
              {server.name}
            </div>
            <StatusBadge ok={ready && server.skill_enabled} label={statusLabel} />
          </div>
        </div>
        <Link
          to={skillsPath}
          className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium transition ${
            server.skill_enabled
              ? 'bg-indigo-100 text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-500/15 dark:text-indigo-300 dark:hover:bg-indigo-500/25'
              : 'bg-amber-100 text-amber-700 hover:bg-amber-200 dark:bg-amber-500/15 dark:text-amber-300 dark:hover:bg-amber-500/25'
          }`}
          title="Enable or disable this integration on the Skills page"
        >
          {server.skill_enabled ? 'Enabled · Skills' : 'Disabled · enable on Skills'}
        </Link>
      </div>

      <div className="mt-4 space-y-3">
        {/* Access mode */}
        <div>
          <span className={labelClass}>Access mode</span>
          <div className="inline-flex rounded-xl border border-gray-200 dark:border-gray-700 p-0.5">
            {(['read', 'readwrite'] as MCPMode[]).map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => setMode(m)}
                className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                  mode === m
                    ? 'bg-indigo-600 text-white dark:bg-indigo-500'
                    : 'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/60'
                }`}
              >
                {m === 'read' ? 'Read-only' : 'Read & write'}
              </button>
            ))}
          </div>
        </div>

        {/* Token (token-auth providers only) */}
        {!isOAuth && (
          <div>
            <label className={labelClass}>API token</label>
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={
                server.configured
                  ? `Saved (${server.token_mask}) — leave blank to keep`
                  : 'Paste the API token'
              }
              autoComplete="off"
              className={inputClass}
            />
          </div>
        )}

        {/* Endpoint (advanced) */}
        <div>
          <button
            type="button"
            onClick={() => setShowEndpoint((v) => !v)}
            className="text-xs font-medium text-indigo-600 dark:text-indigo-400 hover:underline"
          >
            {showEndpoint ? 'Hide' : 'Advanced'} — endpoint
          </button>
          {showEndpoint && (
            <input
              type="text"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              placeholder={server.default_endpoint}
              className={`${inputClass} mt-2`}
            />
          )}
        </div>
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        {isOAuth ? (
          server.connected ? (
            <button
              type="button"
              onClick={disconnect}
              disabled={busy}
              className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-2 text-sm font-medium text-red-600 dark:text-red-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 disabled:opacity-50"
            >
              Disconnect
            </button>
          ) : (
            <button
              type="button"
              onClick={connect}
              disabled={busy}
              className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
            >
              Connect with OAuth
            </button>
          )
        ) : null}
        <button
          type="button"
          onClick={save}
          disabled={busy}
          className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
        >
          Save
        </button>
        <button
          type="button"
          onClick={test}
          disabled={testing || (isOAuth && !server.connected)}
          className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 transition hover:bg-gray-50 dark:hover:bg-gray-800/60 disabled:opacity-50"
        >
          {testing ? 'Testing…' : 'Test connection'}
        </button>
        {!isOAuth && server.configured && (
          <button
            type="button"
            onClick={clearToken}
            disabled={busy}
            className="rounded-xl px-3 py-2 text-sm font-medium text-red-600 dark:text-red-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 disabled:opacity-50"
          >
            Remove token
          </button>
        )}
        {msg && <span className="text-sm text-gray-500 dark:text-gray-400">{msg}</span>}
      </div>
      {testMsg && (
        <p
          className={`mt-2 text-sm ${
            testMsg.startsWith('✓')
              ? 'text-green-600 dark:text-green-400'
              : 'text-red-600 dark:text-red-400'
          }`}
        >
          {testMsg}
        </p>
      )}
    </div>
  );
}

// NotionMappingCard maps labelled Notion databases ("spaces") to the project so
// the agent knows which database is the task tracker, the issue tracker, etc.
function NotionMappingCard({
  targets,
  onChanged,
}: {
  targets: NotionTarget[];
  onChanged: (d: MCPIntegrations) => void;
}) {
  const [kind, setKind] = useState('task');
  const [databaseId, setDatabaseId] = useState('');
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!kind.trim() || !databaseId.trim()) return;
    setBusy(true);
    setMsg('');
    try {
      onChanged(
        await setNotionTarget({
          kind: kind.trim(),
          database_id: databaseId.trim(),
          name: name.trim(),
          url: url.trim(),
        }),
      );
      setDatabaseId('');
      setName('');
      setUrl('');
      setMsg('Saved');
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setBusy(false);
    }
  };

  const remove = async (k: string) => {
    setBusy(true);
    try {
      onChanged(await deleteNotionTarget(k));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Failed to remove');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-50">Notion databases</h2>
      <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        Map a Notion database to a label so the assistant targets the right one — e.g. a task
        tracker and an issue tracker in separate spaces. The database id is the 32-character id in
        the database URL.
      </p>

      {targets.length > 0 && (
        <ul className="mt-4 space-y-2">
          {targets.map((t) => (
            <li
              key={t.kind}
              className="flex items-center justify-between rounded-xl border border-gray-200 dark:border-gray-700 px-3 py-2"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="rounded-full bg-indigo-100 dark:bg-indigo-500/15 px-2 py-0.5 text-xs font-medium text-indigo-700 dark:text-indigo-300">
                    {t.kind}
                  </span>
                  <span className="truncate text-sm font-medium text-gray-900 dark:text-gray-50">
                    {t.name || t.database_id}
                  </span>
                </div>
                <div className="mt-0.5 truncate font-mono text-xs text-gray-400 dark:text-gray-500">
                  {t.database_id}
                </div>
              </div>
              <button
                type="button"
                onClick={() => remove(t.kind)}
                disabled={busy}
                className="ml-3 shrink-0 rounded-lg px-2 py-1 text-xs font-medium text-red-600 dark:text-red-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 disabled:opacity-50"
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}

      <form onSubmit={add} className="mt-4 grid gap-3 sm:grid-cols-2">
        <div>
          <label className={labelClass}>Label</label>
          <input
            type="text"
            list="notion-kinds"
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            placeholder="task"
            className={inputClass}
          />
          <datalist id="notion-kinds">
            <option value="task" />
            <option value="issue" />
          </datalist>
        </div>
        <div>
          <label className={labelClass}>Database id</label>
          <input
            type="text"
            value={databaseId}
            onChange={(e) => setDatabaseId(e.target.value)}
            placeholder="1a2b3c…"
            className={inputClass}
          />
        </div>
        <div>
          <label className={labelClass}>Display name (optional)</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Tasks"
            className={inputClass}
          />
        </div>
        <div>
          <label className={labelClass}>URL (optional)</label>
          <input
            type="text"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://notion.so/…"
            className={inputClass}
          />
        </div>
        <div className="sm:col-span-2 flex items-center gap-2">
          <button
            type="submit"
            disabled={busy}
            className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
          >
            Save mapping
          </button>
          {msg && <span className="text-sm text-gray-500 dark:text-gray-400">{msg}</span>}
        </div>
      </form>
    </div>
  );
}

// IntegrationsMCP is the MCP servers settings sub-page: a card per provider
// (Cloudflare uses an API token; Notion & Railway use OAuth), plus the Notion
// database mapping. Enablement is per project via each provider's skill.
export function IntegrationsMCP() {
  const { projectPath } = useProjects();
  const [data, setData] = useState<MCPIntegrations | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    const refresh = () => {
      getMCPIntegrations()
        .then((d) => {
          if (active) {
            setData(d);
            setError('');
          }
        })
        .catch((e) => {
          if (active) setError(e instanceof Error ? e.message : 'Failed to load MCP integrations');
        })
        .finally(() => {
          if (active) setLoading(false);
        });
    };
    refresh();
    // Refresh when returning from an OAuth authorization tab.
    window.addEventListener('focus', refresh);
    return () => {
      active = false;
      window.removeEventListener('focus', refresh);
    };
  }, []);

  const notionConnected = data?.servers.some((s) => s.slug === 'notion' && s.connected) ?? false;
  const skillsPath = projectPath('settings/project/skills');

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
          MCP servers
        </h1>
        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
          Connect Model Context Protocol servers so the assistant can use their tools. Turn each one
          on per project on the{' '}
          <Link to={skillsPath} className="text-indigo-600 dark:text-indigo-400 hover:underline">
            Skills
          </Link>{' '}
          page, then add credentials here. Cloudflare uses an API token; Notion and Railway use
          OAuth.
        </p>
      </div>

      <div className="mt-6 space-y-6">
        {loading ? (
          <SkeletonFormCard fields={3} />
        ) : error ? (
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        ) : data ? (
          <>
            {data.servers.map((s) => (
              <MCPServerCard key={s.slug} server={s} onChanged={setData} skillsPath={skillsPath} />
            ))}
            {notionConnected && (
              <NotionMappingCard targets={data.notion_targets} onChanged={setData} />
            )}
          </>
        ) : null}
      </div>
    </div>
  );
}
