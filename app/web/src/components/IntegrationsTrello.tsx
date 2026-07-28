import { useState, useEffect, useCallback } from 'react';
import { Link } from 'react-router-dom';
import {
  getIntegrations,
  setTrelloCreds,
  setTrelloBoard,
  getTrelloWorkspaces,
  getTrelloBoards,
  getTrelloAvailableWorkspaces,
  getTrelloAvailableBoards,
  attachTrelloWorkspace,
  attachTrelloBoard,
  deleteTrelloWorkspace,
  deleteTrelloBoard,
} from '../api/client';
import { useProjects } from '../contexts/project';
import type {
  Integrations as IntegrationsData,
  TrelloWorkspaceLink,
  TrelloBoardLink,
  TrelloRemoteWorkspace,
  TrelloRemoteBoard,
} from '../types';
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

// A linked workspace together with its linked boards.
type WorkspaceGroup = { ws: TrelloWorkspaceLink; boards: TrelloBoardLink[] };

// TrelloWorkspacesCard manages this project's Trello links: a project can link
// many workspaces, and each workspace many boards, all chosen live from the
// Trello API. One board is marked "Active" — the single board the Trello skills
// read and write (persisted via the legacy workspace/board mapping so existing
// skills keep working).
function TrelloWorkspacesCard({
  credsConfigured,
  activeBoardId,
  onSetActive,
}: {
  credsConfigured: boolean;
  activeBoardId: string;
  onSetActive: (workspaceTrelloId: string, boardTrelloId: string) => Promise<void>;
}) {
  const [groups, setGroups] = useState<WorkspaceGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [msg, setMsg] = useState('');

  // Add-workspace picker.
  const [wsPickerOpen, setWsPickerOpen] = useState(false);
  const [availableWs, setAvailableWs] = useState<TrelloRemoteWorkspace[] | null>(null);
  const [wsBusy, setWsBusy] = useState(false);

  // Add-board picker (at most one open, keyed by the workspace's DB id).
  const [boardPickerFor, setBoardPickerFor] = useState<number | null>(null);
  const [availableBoards, setAvailableBoards] = useState<TrelloRemoteBoard[] | null>(null);
  const [boardBusy, setBoardBusy] = useState(false);

  // Loads (and reloads) the linked workspaces + their boards. Written as a
  // promise chain (setState only in .then/.catch/.finally callbacks) so it is
  // safe to call from an effect; `loading` starts true for the initial paint and
  // is cleared in the finally below.
  const load = useCallback(() => {
    return getTrelloWorkspaces()
      .then((wss) =>
        Promise.all(wss.map((ws) => getTrelloBoards(ws.id).then((boards) => ({ ws, boards })))),
      )
      .then((withBoards) => {
        setGroups(withBoards);
        setError('');
      })
      .catch((e) => {
        setError(e instanceof Error ? e.message : 'Failed to load Trello workspaces');
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const fail = (e: unknown, fallback: string) => setMsg(e instanceof Error ? e.message : fallback);

  // Open the workspace picker, fetching what's available live from Trello.
  const openWsPicker = async () => {
    setMsg('');
    setWsPickerOpen(true);
    if (availableWs === null) {
      try {
        setAvailableWs(await getTrelloAvailableWorkspaces());
      } catch (e) {
        fail(e, 'Failed to load workspaces from Trello');
        setWsPickerOpen(false);
      }
    }
  };

  const addWorkspace = async (trelloId: string) => {
    const remote = (availableWs ?? []).find((w) => w.id === trelloId);
    if (!remote) return;
    setWsBusy(true);
    setMsg('');
    try {
      await attachTrelloWorkspace({
        id: remote.id,
        name: remote.display_name || remote.name,
        url: remote.url,
      });
      setWsPickerOpen(false);
      await load();
    } catch (e) {
      fail(e, 'Failed to link workspace');
    } finally {
      setWsBusy(false);
    }
  };

  const removeWorkspace = async (id: number) => {
    setMsg('');
    try {
      await deleteTrelloWorkspace(id);
      await load();
    } catch (e) {
      fail(e, 'Failed to unlink workspace');
    }
  };

  // Open the board picker for a workspace, fetching its boards live from Trello.
  const openBoardPicker = async (workspaceId: number) => {
    setMsg('');
    setBoardPickerFor(workspaceId);
    setAvailableBoards(null);
    try {
      setAvailableBoards(await getTrelloAvailableBoards(workspaceId));
    } catch (e) {
      fail(e, 'Failed to load boards from Trello');
      setBoardPickerFor(null);
    }
  };

  const addBoard = async (workspaceId: number, trelloId: string) => {
    const remote = (availableBoards ?? []).find((b) => b.id === trelloId);
    if (!remote) return;
    setBoardBusy(true);
    setMsg('');
    try {
      await attachTrelloBoard(workspaceId, { id: remote.id, name: remote.name, url: remote.url });
      setBoardPickerFor(null);
      await load();
    } catch (e) {
      fail(e, 'Failed to link board');
    } finally {
      setBoardBusy(false);
    }
  };

  const removeBoard = async (id: number) => {
    setMsg('');
    try {
      await deleteTrelloBoard(id);
      await load();
    } catch (e) {
      fail(e, 'Failed to unlink board');
    }
  };

  const makeActive = async (wsTrelloId: string, boardTrelloId: string) => {
    setMsg('');
    try {
      await onSetActive(wsTrelloId, boardTrelloId);
    } catch (e) {
      fail(e, 'Failed to set active board');
    }
  };

  const linkedWsIds = new Set(groups.map((g) => g.ws.trello_id));
  const pickableWs = (availableWs ?? []).filter((w) => !linkedWsIds.has(w.id));

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
      <div className="mb-1 flex items-center gap-3">
        <TrelloIcon />
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-50">
          Workspaces &amp; boards
        </h2>
      </div>
      <p className="mb-4 text-xs text-gray-400 dark:text-gray-500">
        Link one or more Trello workspaces to this project, and any number of boards under each. The
        board marked <span className="font-medium">Active</span> is the one the Trello skills read
        and write — tasks land on its <span className="font-medium">Backlog/Todo</span> list, bugs
        on its <span className="font-medium">Bug</span> list, ideas on its{' '}
        <span className="font-medium">Ideas</span> list (matched by name).
      </p>

      {!credsConfigured ? (
        <p className="rounded-xl bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">
          Add your Trello credentials above to link workspaces and boards.
        </p>
      ) : loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading…</p>
      ) : error ? (
        <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
      ) : (
        <div className="space-y-4">
          {groups.length === 0 && (
            <p className="text-sm text-gray-500 dark:text-gray-400">No workspaces linked yet.</p>
          )}

          {groups.map(({ ws, boards }) => (
            <div key={ws.id} className="rounded-xl border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-gray-700">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-gray-900 dark:text-gray-50">
                    {ws.name || ws.trello_id}
                  </p>
                  <p className="truncate text-xs text-gray-400 dark:text-gray-500">
                    {boards.length} {boards.length === 1 ? 'board' : 'boards'}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => removeWorkspace(ws.id)}
                  className="shrink-0 rounded-lg px-2 py-1 text-xs font-medium text-red-600 transition hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/15"
                >
                  Remove
                </button>
              </div>

              <div className="space-y-2 p-4">
                {boards.length === 0 && (
                  <p className="text-xs text-gray-400 dark:text-gray-500">No boards linked yet.</p>
                )}
                {boards.map((b) => {
                  const active = b.trello_id === activeBoardId && activeBoardId !== '';
                  return (
                    <div
                      key={b.id}
                      className="flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-3 py-2 dark:bg-gray-900/50"
                    >
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-sm text-gray-800 dark:text-gray-200">
                          {b.name || b.trello_id}
                        </span>
                        {active && (
                          <span className="shrink-0 rounded-full bg-indigo-100 px-2 py-0.5 text-[11px] font-medium text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300">
                            Active
                          </span>
                        )}
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        {!active && (
                          <button
                            type="button"
                            onClick={() => makeActive(ws.trello_id, b.trello_id)}
                            className="rounded-lg px-2 py-1 text-xs font-medium text-indigo-700 transition hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-500/15"
                          >
                            Set active
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => removeBoard(b.id)}
                          aria-label="Remove board"
                          className="rounded-lg px-2 py-1 text-xs font-medium text-gray-400 transition hover:bg-red-50 hover:text-red-600 dark:text-gray-500 dark:hover:bg-red-500/15 dark:hover:text-red-400"
                        >
                          Remove
                        </button>
                      </div>
                    </div>
                  );
                })}

                {boardPickerFor === ws.id ? (
                  <div className="flex items-center gap-2">
                    <select
                      autoFocus
                      disabled={boardBusy || availableBoards === null}
                      defaultValue=""
                      onChange={(e) => e.target.value && addBoard(ws.id, e.target.value)}
                      className={inputClass}
                    >
                      <option value="" disabled>
                        {availableBoards === null ? 'Loading boards…' : 'Select a board…'}
                      </option>
                      {(availableBoards ?? [])
                        .filter((rb) => !boards.some((b) => b.trello_id === rb.id))
                        .map((rb) => (
                          <option key={rb.id} value={rb.id}>
                            {rb.name}
                          </option>
                        ))}
                    </select>
                    <button
                      type="button"
                      onClick={() => setBoardPickerFor(null)}
                      className="shrink-0 rounded-lg px-2 py-1 text-xs font-medium text-gray-500 transition hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-700"
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => openBoardPicker(ws.id)}
                    className="text-sm font-medium text-indigo-700 transition hover:text-indigo-800 dark:text-indigo-400 dark:hover:text-indigo-300"
                  >
                    + Add board
                  </button>
                )}
              </div>
            </div>
          ))}

          {wsPickerOpen ? (
            <div className="flex items-center gap-2">
              <select
                autoFocus
                disabled={wsBusy || availableWs === null}
                defaultValue=""
                onChange={(e) => e.target.value && addWorkspace(e.target.value)}
                className={inputClass}
              >
                <option value="" disabled>
                  {availableWs === null
                    ? 'Loading workspaces…'
                    : pickableWs.length === 0
                      ? 'All workspaces already linked'
                      : 'Select a workspace…'}
                </option>
                {pickableWs.map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.display_name || w.name}
                  </option>
                ))}
              </select>
              <button
                type="button"
                onClick={() => setWsPickerOpen(false)}
                className="shrink-0 rounded-lg px-2 py-1 text-xs font-medium text-gray-500 transition hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-700"
              >
                Cancel
              </button>
            </div>
          ) : (
            <button
              type="button"
              onClick={openWsPicker}
              className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
            >
              Link a workspace
            </button>
          )}

          {msg && <p className="text-sm text-red-600 dark:text-red-400">{msg}</p>}
        </div>
      )}
    </div>
  );
}

// Trello integration detail page. Mirrors the WhatsApp integration detail page:
// a back-link to the integrations list, a header, then the credentials card and
// the workspace/board manager.
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
          <>
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
            <TrelloWorkspacesCard
              credsConfigured={data.trello_configured}
              activeBoardId={data.trello_board_id}
              onSetActive={async (workspaceId, boardId) => {
                const d = await setTrelloBoard(workspaceId, boardId);
                setData(d);
              }}
            />
          </>
        ) : null}
      </div>
    </div>
  );
}
