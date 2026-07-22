import { useState, useEffect, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  listHikes,
  createHike,
  updateHike,
  deleteHike,
  getHikeOptions,
  listHikeTracks,
} from '../api/client';
import type { Hike, HikePayload, HikeOptions, HikeNameOption } from '../types';
import { Toggle } from './ui/Toggle';
import { SkeletonListRow } from './ui/Skeleton';
import { Modal } from './ui/Modal';
import { DatePicker } from './DatePicker';
import { StatTile } from './dashboard/parts';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:ring-indigo-500/30';

function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function emptyForm(): HikePayload {
  return {
    mountain: '',
    up_track: '',
    down_track: '',
    camped: false,
    days: 1,
    nights: 0,
    hiked_on: todayISO(),
    participants: [],
  };
}

function toPayload(h: Hike): HikePayload {
  return {
    mountain: h.mountain,
    up_track: h.up_track,
    down_track: h.down_track,
    camped: h.camped,
    days: h.days,
    nights: h.nights,
    hiked_on: h.hiked_on,
    participants: h.participants,
  };
}

function formatDate(iso: string): string {
  const d = new Date(iso + 'T00:00:00');
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

// The one-line detail shown under a hike's mountain + date: trails, duration,
// camping, and companions, each omitted when it has nothing to say.
function summarize(h: Hike): string {
  const parts: string[] = [];
  if (h.up_track || h.down_track) {
    parts.push(`↑ ${h.up_track || '—'} · ↓ ${h.down_track || '—'}`);
  }
  if (h.days > 0 || h.nights > 0) {
    parts.push(`${h.days}D/${h.nights}N`);
  }
  if (h.camped) parts.push('⛺ Camped');
  if (h.participants.length > 0) parts.push(`with ${h.participants.join(', ')}`);
  return parts.join('  ·  ');
}

// A labelled horizontal bar (count relative to the biggest in its group) used
// by the analytics ranking cards — peaks, years, companions.
function BarRow({
  label,
  value,
  max,
  suffix = '',
}: {
  label: string;
  value: number;
  max: number;
  suffix?: string;
}) {
  return (
    <div className="flex items-center gap-3">
      <div
        className="w-28 shrink-0 truncate text-sm font-medium text-gray-700 dark:text-gray-300"
        title={label}
      >
        {label}
      </div>
      <div className="h-5 flex-1 overflow-hidden rounded-md bg-gray-100 dark:bg-gray-700">
        <div
          className="h-full rounded-md bg-indigo-600 transition-all dark:bg-indigo-500"
          style={{ width: `${max ? Math.max((value / max) * 100, 6) : 0}%` }}
        />
      </div>
      <div className="w-14 shrink-0 text-right text-xs font-medium tabular-nums text-gray-500 dark:text-gray-400">
        {value}
        {suffix}
      </div>
    </div>
  );
}

// A titled analytics card matching the bucket-list page's panel style.
function AnalyticsCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div className="text-sm font-semibold text-gray-900 dark:text-gray-100">{title}</div>
      {subtitle && <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{subtitle}</p>}
      <div className="mt-3">{children}</div>
    </div>
  );
}

export function Hikes() {
  const [hikes, setHikes] = useState<Hike[]>([]);
  const [options, setOptions] = useState<HikeOptions>({ mountains: [], hikers: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editing, setEditing] = useState<{ id: number | null; form: HikePayload } | null>(null);

  // Persist the search term in the URL so it survives reloads and is shareable,
  // matching the Reminders/Logs filter convention.
  const [searchParams, setSearchParams] = useSearchParams();
  const query = searchParams.get('q') ?? '';
  const setQuery = (next: string) => {
    const sp = new URLSearchParams(searchParams);
    if (next.trim()) sp.set('q', next);
    else sp.delete('q');
    setSearchParams(sp, { replace: true });
  };

  useEffect(() => {
    let active = true;
    Promise.all([listHikes(), getHikeOptions()])
      .then(([hs, opts]) => {
        if (!active) return;
        setHikes(hs);
        setOptions(opts);
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load hikes'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => {
    const [hs, opts] = await Promise.all([listHikes(), getHikeOptions()]);
    setHikes(hs);
    setOptions(opts);
  };

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return hikes;
    return hikes.filter((h) => {
      const haystack = [h.mountain, h.up_track, h.down_track, ...h.participants]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [hikes, query]);

  // Aggregate stats for the analytics section, recomputed from the full (not
  // search-filtered) log so the numbers reflect your whole hiking history.
  const stats = useMemo(() => {
    const mountains = new Map<string, number>();
    const companions = new Map<string, number>();
    const years = new Map<number, number>();
    let days = 0;
    let nights = 0;
    let camped = 0;
    const currentYear = new Date().getFullYear();
    let thisYear = 0;

    for (const h of hikes) {
      days += h.days || 0;
      nights += h.nights || 0;
      if (h.camped) camped += 1;
      if (h.mountain) mountains.set(h.mountain, (mountains.get(h.mountain) ?? 0) + 1);
      for (const p of h.participants) companions.set(p, (companions.get(p) ?? 0) + 1);
      const y = Number(h.hiked_on.slice(0, 4));
      if (!Number.isNaN(y) && y > 0) {
        years.set(y, (years.get(y) ?? 0) + 1);
        if (y === currentYear) thisYear += 1;
      }
    }

    const rank = (m: Map<string, number>, n: number) =>
      [...m.entries()]
        .map(([name, count]) => ({ name, count }))
        .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
        .slice(0, n);

    const topMountains = rank(mountains, 6);
    const topCompanions = rank(companions, 6);
    const byYear = [...years.entries()]
      .map(([year, count]) => ({ year, count }))
      .sort((a, b) => b.year - a.year);

    return {
      total: hikes.length,
      peaks: mountains.size,
      days,
      nights,
      camped,
      companions: companions.size,
      thisYear,
      topMountains,
      topCompanions,
      byYear,
    };
  }, [hikes]);

  const remove = async (h: Hike) => {
    setBusyId(h.id);
    setError('');
    try {
      await deleteHike(h.id);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete hike');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Hikes
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Every hiking trip you’ve logged — mountain, trails, duration, and who came along. Add,
            edit, or remove entries here or through chat; both share the same log.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <button
            type="button"
            onClick={() => setEditing({ id: null, form: emptyForm() })}
            className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
          >
            + New hike
          </button>
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <Modal
        open={editing !== null}
        onClose={() => setEditing(null)}
        title={editing?.id != null ? 'Edit hike' : 'New hike'}
      >
        {editing && (
          <HikeForm
            initial={editing.form}
            options={options}
            onCancel={() => setEditing(null)}
            onSave={async (payload) => {
              if (editing.id === null) await createHike(payload);
              else await updateHike(editing.id, payload);
              await reload();
              setEditing(null);
            }}
          />
        )}
      </Modal>

      {/* Analytics: at-a-glance totals plus rankings across your whole log */}
      {!loading && hikes.length > 0 && (
        <div className="mt-4 space-y-3">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
            <StatTile
              label="Hikes"
              value={String(stats.total)}
              sub={stats.thisYear > 0 ? `${stats.thisYear} this year` : undefined}
            />
            <StatTile
              label="Peaks"
              value={String(stats.peaks)}
              sub={stats.topMountains[0] ? `Top: ${stats.topMountains[0].name}` : undefined}
            />
            <StatTile
              label="Days on trail"
              value={String(stats.days)}
              sub={stats.nights > 0 ? `${stats.nights} nights out` : undefined}
            />
            <StatTile
              label="Camped trips"
              value={String(stats.camped)}
              sub={
                stats.total > 0
                  ? `${Math.round((stats.camped / stats.total) * 100)}% of hikes`
                  : undefined
              }
            />
            <StatTile
              label="Companions"
              value={String(stats.companions)}
              sub={stats.topCompanions[0] ? `Most: ${stats.topCompanions[0].name}` : undefined}
            />
          </div>

          <div className="grid gap-3 lg:grid-cols-2">
            {stats.topMountains.length > 0 && (
              <AnalyticsCard title="Most-climbed peaks" subtitle="Mountains you return to most.">
                <div className="space-y-2.5">
                  {stats.topMountains.map((m) => (
                    <BarRow
                      key={m.name}
                      label={m.name}
                      value={m.count}
                      max={stats.topMountains[0].count}
                      suffix="×"
                    />
                  ))}
                </div>
              </AnalyticsCard>
            )}

            {stats.byYear.length > 0 && (
              <AnalyticsCard title="Hikes by year" subtitle="Trips logged each year.">
                <div className="space-y-2.5">
                  {stats.byYear.map((y) => (
                    <BarRow
                      key={y.year}
                      label={String(y.year)}
                      value={y.count}
                      max={Math.max(...stats.byYear.map((r) => r.count))}
                    />
                  ))}
                </div>
              </AnalyticsCard>
            )}
          </div>

          {stats.topCompanions.length > 0 && (
            <AnalyticsCard title="Top companions" subtitle="Who’s joined you on the most trips.">
              <div className="space-y-2.5">
                {stats.topCompanions.map((c) => (
                  <BarRow
                    key={c.name}
                    label={c.name}
                    value={c.count}
                    max={stats.topCompanions[0].count}
                    suffix="×"
                  />
                ))}
              </div>
            </AnalyticsCard>
          )}
        </div>
      )}

      {!loading && hikes.length > 0 && (
        <div className="mt-5">
          <div className="relative max-w-sm">
            <svg
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M21 21l-4.35-4.35M17 11a6 6 0 11-12 0 6 6 0 0112 0z"
              />
            </svg>
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search mountain, trail, or companion"
              className={`${inputClass} pl-9`}
            />
          </div>
        </div>
      )}

      {loading ? (
        <div className="mt-5 space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <SkeletonListRow key={i} trailingWidth="w-28" />
          ))}
        </div>
      ) : hikes.length === 0 ? (
        <div className="mt-6 rounded-2xl border border-dashed border-gray-300 bg-white p-8 text-center dark:border-gray-700 dark:bg-gray-800">
          <p className="text-sm font-medium text-gray-900 dark:text-gray-50">No hikes logged yet</p>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Add your first trip to start building your hiking log.
          </p>
        </div>
      ) : visible.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">No hikes match “{query}”.</p>
      ) : (
        <div className="mt-4 space-y-2">
          {visible.map((h) => (
            <div
              key={h.id}
              className="flex items-start gap-4 rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
                  <span className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                    {h.mountain}
                  </span>
                  <span className="text-xs text-gray-500 dark:text-gray-400">
                    {formatDate(h.hiked_on)}
                  </span>
                </div>
                {summarize(h) && (
                  <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{summarize(h)}</p>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-3">
                <button
                  type="button"
                  onClick={() => setEditing({ id: h.id, form: toPayload(h) })}
                  className="text-sm font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
                >
                  Edit
                </button>
                <button
                  type="button"
                  disabled={busyId === h.id}
                  onClick={() => remove(h)}
                  className="text-sm font-medium text-red-600 hover:text-red-700 disabled:opacity-50 dark:text-red-400 dark:hover:text-red-300"
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function HikeForm({
  initial,
  options,
  onSave,
  onCancel,
}: {
  initial: HikePayload;
  options: HikeOptions;
  onSave: (payload: HikePayload) => Promise<void>;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<HikePayload>(initial);
  const [participantDraft, setParticipantDraft] = useState('');
  const [trackOptions, setTrackOptions] = useState<HikeNameOption[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const set = <K extends keyof HikePayload>(k: K, v: HikePayload[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  // Once the typed mountain matches a known one, offer its recorded trails as
  // up/down suggestions. A brand-new mountain simply has none yet.
  const matchedMountain = useMemo(
    () =>
      options.mountains.find((m) => m.name.toLowerCase() === form.mountain.trim().toLowerCase()),
    [options.mountains, form.mountain],
  );
  useEffect(() => {
    let active = true;
    // A brand-new (unmatched) mountain has no recorded trails yet, so resolve to
    // an empty list; only ever set state from the async callback so this effect
    // never triggers a synchronous cascading render.
    const pending = matchedMountain ? listHikeTracks(matchedMountain.id) : Promise.resolve([]);
    pending.then((ts) => active && setTrackOptions(ts)).catch(() => active && setTrackOptions([]));
    return () => {
      active = false;
    };
  }, [matchedMountain]);

  const addParticipant = (raw: string) => {
    const name = raw.trim();
    if (!name) return;
    setForm((f) =>
      f.participants.some((p) => p.toLowerCase() === name.toLowerCase())
        ? f
        : { ...f, participants: [...f.participants, name] },
    );
    setParticipantDraft('');
  };
  const removeParticipant = (name: string) =>
    setForm((f) => ({ ...f, participants: f.participants.filter((p) => p !== name) }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.mountain.trim()) {
      setError('Mountain is required');
      return;
    }
    setSaving(true);
    setError('');
    // Fold any half-typed participant into the list so it isn't silently lost.
    const payload: HikePayload = participantDraft.trim()
      ? {
          ...form,
          participants: form.participants.some(
            (p) => p.toLowerCase() === participantDraft.trim().toLowerCase(),
          )
            ? form.participants
            : [...form.participants, participantDraft.trim()],
        }
      : form;
    try {
      await onSave(payload);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <form onSubmit={submit}>
      <div className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Mountain
          </label>
          <input
            list="hike-mountains"
            value={form.mountain}
            onChange={(e) => set('mountain', e.target.value)}
            placeholder="e.g. Rinjani"
            className={inputClass}
          />
          <datalist id="hike-mountains">
            {options.mountains.map((m) => (
              <option key={m.id} value={m.name} />
            ))}
          </datalist>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Date hiked
          </label>
          <DatePicker value={form.hiked_on} onChange={(v) => set('hiked_on', v)} max={todayISO()} />
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Up trail
            </label>
            <input
              list="hike-tracks"
              value={form.up_track}
              onChange={(e) => set('up_track', e.target.value)}
              placeholder="Optional"
              className={inputClass}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Down trail
            </label>
            <input
              list="hike-tracks"
              value={form.down_track}
              onChange={(e) => set('down_track', e.target.value)}
              placeholder="Optional"
              className={inputClass}
            />
          </div>
          <datalist id="hike-tracks">
            {trackOptions.map((t) => (
              <option key={t.id} value={t.name} />
            ))}
          </datalist>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Days
            </label>
            <input
              type="number"
              min={0}
              max={365}
              value={form.days}
              onChange={(e) => set('days', Math.max(0, Number(e.target.value)))}
              className={inputClass}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Nights
            </label>
            <input
              type="number"
              min={0}
              max={365}
              value={form.nights}
              onChange={(e) => set('nights', Math.max(0, Number(e.target.value)))}
              className={inputClass}
            />
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Toggle on={form.camped} onClick={() => set('camped', !form.camped)} />
          <span className="text-sm text-gray-600 dark:text-gray-300">Camped overnight</span>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Participants
          </label>
          {form.participants.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-2">
              {form.participants.map((p) => (
                <span
                  key={p}
                  className="inline-flex items-center gap-1.5 rounded-full bg-indigo-50 py-1 pl-3 pr-1.5 text-sm font-medium text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300"
                >
                  {p}
                  <button
                    type="button"
                    onClick={() => removeParticipant(p)}
                    aria-label={`Remove ${p}`}
                    className="flex h-4 w-4 items-center justify-center rounded-full text-indigo-500 transition hover:bg-indigo-200 hover:text-indigo-800 dark:text-indigo-300 dark:hover:bg-indigo-500/30 dark:hover:text-indigo-100"
                  >
                    <svg
                      className="h-3 w-3"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth={2}
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" d="M6 6l12 12M18 6L6 18" />
                    </svg>
                  </button>
                </span>
              ))}
            </div>
          )}
          <div className="flex items-center gap-2">
            <input
              list="hike-hikers"
              value={participantDraft}
              onChange={(e) => setParticipantDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  addParticipant(participantDraft);
                }
              }}
              placeholder="Add a companion"
              className={inputClass}
            />
            <datalist id="hike-hikers">
              {options.hikers.map((h) => (
                <option key={h.id} value={h.name} />
              ))}
            </datalist>
            <button
              type="button"
              onClick={() => addParticipant(participantDraft)}
              className="shrink-0 rounded-xl border border-gray-200 px-3 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800/60"
            >
              Add
            </button>
          </div>
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <div className="mt-5 flex items-center gap-3">
        <button
          type="submit"
          disabled={saving}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
