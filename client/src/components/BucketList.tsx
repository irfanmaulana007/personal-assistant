import { useState, useEffect, useMemo } from 'react';
import {
  listBucketItems,
  createBucketItem,
  updateBucketItem,
  setBucketItemDone,
  setBucketItemResolution,
  deleteBucketItem,
} from '../api/client';
import type { BucketItem, BucketCategory } from '../types';
import { Skeleton } from './ui/Skeleton';

const inputClass =
  'w-full rounded-xl border border-gray-200 dark:border-gray-700 dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 outline-none transition focus:border-indigo-500 dark:focus:border-indigo-400 focus:ring-2 focus:ring-indigo-200 dark:focus:ring-indigo-500/30';

const CURRENT_YEAR = new Date().getFullYear();

interface CategoryMeta {
  key: BucketCategory;
  label: string;
  dot: string; // solid swatch, used in legends/breakdown
  badge: string; // subtle pill used on item rows (light + dark)
}

// The five fixed buckets. Order here drives grouping and breakdown order.
const CATEGORIES: CategoryMeta[] = [
  {
    key: 'self_improvement',
    label: 'Self Improvement',
    dot: 'bg-indigo-500',
    badge: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300',
  },
  {
    key: 'learning',
    label: 'Learning',
    dot: 'bg-rose-500',
    badge: 'bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300',
  },
  {
    key: 'hiking',
    label: 'Hiking Destination',
    dot: 'bg-emerald-500',
    badge: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
  },
  {
    key: 'country',
    label: 'Country Destination',
    dot: 'bg-sky-500',
    badge: 'bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300',
  },
  {
    key: 'local',
    label: 'Local Place Destination',
    dot: 'bg-amber-500',
    badge: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  },
  {
    key: 'other',
    label: 'Other',
    dot: 'bg-gray-400 dark:bg-gray-500',
    badge: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
  },
];

const CATEGORY_BY_KEY: Record<BucketCategory, CategoryMeta> = CATEGORIES.reduce(
  (acc, c) => {
    acc[c.key] = c;
    return acc;
  },
  {} as Record<BucketCategory, CategoryMeta>,
);

function catMeta(key: BucketCategory): CategoryMeta {
  return CATEGORY_BY_KEY[key] ?? CATEGORY_BY_KEY.other;
}

// A circular check control used for each bucket-list item.
function CheckCircle({
  done,
  busy,
  onClick,
}: {
  done: boolean;
  busy?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={done}
      disabled={busy}
      onClick={onClick}
      className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 transition disabled:opacity-50 ${
        done
          ? 'border-indigo-600 bg-indigo-600 text-white dark:border-indigo-500 dark:bg-indigo-500'
          : 'border-gray-300 bg-white text-transparent hover:border-indigo-400 dark:border-gray-600 dark:bg-gray-900 dark:hover:border-indigo-400'
      }`}
    >
      <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
      </svg>
    </button>
  );
}

function CategoryBadge({ category }: { category: BucketCategory }) {
  const m = catMeta(category);
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${m.badge}`}
    >
      {m.label}
    </span>
  );
}

// A star toggle that marks / unmarks an item as this year's resolution.
function ResolutionStar({
  active,
  busy,
  onClick,
}: {
  active: boolean;
  busy?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={busy}
      onClick={onClick}
      title={
        active ? `Remove from ${CURRENT_YEAR} resolutions` : `Make it a ${CURRENT_YEAR} resolution`
      }
      aria-pressed={active}
      className={`flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium transition disabled:opacity-50 ${
        active
          ? 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
          : 'text-gray-400 hover:bg-gray-100 hover:text-amber-600 dark:text-gray-500 dark:hover:bg-gray-700 dark:hover:text-amber-400'
      }`}
    >
      <svg
        className="h-3.5 w-3.5"
        viewBox="0 0 24 24"
        fill={active ? 'currentColor' : 'none'}
        stroke="currentColor"
        strokeWidth={active ? 0 : 1.8}
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M11.48 3.5a.56.56 0 011.04 0l2.12 4.3a.56.56 0 00.42.3l4.74.69c.5.07.7.69.34 1.04l-3.43 3.34a.56.56 0 00-.16.5l.81 4.72c.09.5-.44.88-.88.64l-4.24-2.23a.56.56 0 00-.52 0L7.32 21.9c-.44.24-.97-.14-.88-.64l.81-4.72a.56.56 0 00-.16-.5L3.66 12.7a.56.56 0 01.34-1.04l4.74-.69a.56.56 0 00.42-.3l2.12-4.3z"
        />
      </svg>
      {active ? CURRENT_YEAR : 'Resolution'}
    </button>
  );
}

// An SVG progress ring — theme colours come from Tailwind classes on the
// <circle> stroke so it flips with light / dark automatically.
function ProgressRing({ pct, size = 96 }: { pct: number; size?: number }) {
  const stroke = 8;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const offset = c - (pct / 100) * c;
  return (
    <svg width={size} height={size} className="shrink-0 -rotate-90">
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        strokeWidth={stroke}
        className="stroke-gray-100 dark:stroke-gray-700"
      />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        strokeWidth={stroke}
        strokeLinecap="round"
        strokeDasharray={c}
        strokeDashoffset={offset}
        className="stroke-indigo-600 transition-all duration-500 dark:stroke-indigo-500"
      />
    </svg>
  );
}

// A labelled horizontal progress bar (done / total).
function ProgressBar({ done, total }: { done: number; total: number }) {
  const pct = total ? Math.round((done / total) * 100) : 0;
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-gray-700">
      <div
        className="h-full rounded-full bg-indigo-600 transition-all dark:bg-indigo-500"
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

export function BucketList() {
  const [items, setItems] = useState<BucketItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editingId, setEditingId] = useState<number | null>(null);

  // Filters.
  const [filterCat, setFilterCat] = useState<BucketCategory | 'all'>('all');
  const [resolutionsOnly, setResolutionsOnly] = useState(false);

  // New-item form state.
  const [newTitle, setNewTitle] = useState('');
  const [newCategory, setNewCategory] = useState<BucketCategory>('self_improvement');
  const [newDescription, setNewDescription] = useState('');
  const [newNote, setNewNote] = useState('');
  const [newResolution, setNewResolution] = useState(false);
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    let active = true;
    listBucketItems()
      .then((gs) => active && setItems(gs))
      .catch(
        (e) => active && setError(e instanceof Error ? e.message : 'Failed to load bucket list'),
      )
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => setItems(await listBucketItems());

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    const title = newTitle.trim();
    if (!title) return;
    setAdding(true);
    setError('');
    try {
      const created = await createBucketItem({
        title,
        description: newDescription.trim(),
        note: newNote.trim(),
        category: newCategory,
      });
      if (newResolution) {
        await setBucketItemResolution(created.id, CURRENT_YEAR);
      }
      setNewTitle('');
      setNewDescription('');
      setNewNote('');
      setNewResolution(false);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add');
    } finally {
      setAdding(false);
    }
  };

  const toggleDone = async (g: BucketItem) => {
    setBusyId(g.id);
    setError('');
    try {
      await setBucketItemDone(g.id, !g.done);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update');
    } finally {
      setBusyId(null);
    }
  };

  const toggleResolution = async (g: BucketItem) => {
    setBusyId(g.id);
    setError('');
    try {
      const next = g.resolution_year === CURRENT_YEAR ? null : CURRENT_YEAR;
      await setBucketItemResolution(g.id, next);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update resolution');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (g: BucketItem) => {
    setBusyId(g.id);
    setError('');
    try {
      await deleteBucketItem(g.id);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete');
    } finally {
      setBusyId(null);
    }
  };

  const saveEdit = async (
    id: number,
    payload: { title: string; description: string; note: string; category: BucketCategory },
  ) => {
    await updateBucketItem(id, {
      title: payload.title.trim(),
      description: payload.description.trim(),
      note: payload.note.trim(),
      category: payload.category,
    });
    await reload();
    setEditingId(null);
  };

  // --- Derived stats ---------------------------------------------------------

  const doneCount = items.filter((g) => g.done).length;
  const overallPct = items.length ? Math.round((doneCount / items.length) * 100) : 0;

  const thisYear = useMemo(() => {
    const res = items.filter((g) => g.resolution_year === CURRENT_YEAR);
    const achieved = res.filter((g) => g.done).length;
    return {
      total: res.length,
      achieved,
      pct: res.length ? Math.round((achieved / res.length) * 100) : 0,
    };
  }, [items]);

  // Resolutions grouped by year (newest first) for the track-record chart.
  const byYear = useMemo(() => {
    const m = new Map<number, { total: number; achieved: number }>();
    for (const g of items) {
      if (g.resolution_year == null) continue;
      const cur = m.get(g.resolution_year) ?? { total: 0, achieved: 0 };
      cur.total += 1;
      if (g.done) cur.achieved += 1;
      m.set(g.resolution_year, cur);
    }
    return [...m.entries()].map(([year, v]) => ({ year, ...v })).sort((a, b) => b.year - a.year);
  }, [items]);

  // Per-category done/total, in fixed category order, skipping empty buckets.
  const byCategory = useMemo(() => {
    return CATEGORIES.map((c) => {
      const inCat = items.filter((g) => g.category === c.key);
      return { ...c, total: inCat.length, done: inCat.filter((g) => g.done).length };
    }).filter((c) => c.total > 0);
  }, [items]);

  // --- Filtering & grouping --------------------------------------------------

  const filtered = useMemo(() => {
    return items.filter((g) => {
      if (resolutionsOnly && g.resolution_year == null) return false;
      if (filterCat !== 'all' && g.category !== filterCat) return false;
      return true;
    });
  }, [items, filterCat, resolutionsOnly]);

  // When no category filter is active, show items grouped by category.
  const groups = useMemo(() => {
    if (filterCat !== 'all') return null;
    return CATEGORIES.map((c) => ({
      meta: c,
      items: filtered.filter((g) => g.category === c.key),
    })).filter((grp) => grp.items.length > 0);
  }, [filtered, filterCat]);

  const maxYearTotal = Math.max(1, ...byYear.map((y) => y.total));

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
          Bucket List
        </h1>
        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
          Everything you want to do in life, sorted into buckets. Flag the ones you want to tackle
          this year as resolutions, and check them off as you go.
        </p>
      </div>

      {/* Overview: this-year resolutions + overall progress */}
      {!loading && items.length > 0 && (
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          {/* This year resolutions */}
          <div className="flex items-center gap-4 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
            <div className="relative">
              <ProgressRing pct={thisYear.pct} />
              <div className="absolute inset-0 flex flex-col items-center justify-center">
                <span className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                  {thisYear.pct}%
                </span>
              </div>
            </div>
            <div className="min-w-0">
              <div className="text-sm font-semibold text-gray-900 dark:text-gray-100">
                {CURRENT_YEAR} Resolutions
              </div>
              {thisYear.total > 0 ? (
                <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
                  {thisYear.achieved} of {thisYear.total} achieved
                </p>
              ) : (
                <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
                  Flag an item with the star to make it a {CURRENT_YEAR} resolution.
                </p>
              )}
            </div>
          </div>

          {/* Overall progress */}
          <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
            <div className="flex items-center justify-between text-sm">
              <span className="font-semibold text-gray-900 dark:text-gray-100">
                Overall progress
              </span>
              <span className="text-gray-500 dark:text-gray-400">
                {doneCount} of {items.length} · {overallPct}%
              </span>
            </div>
            <div className="mt-3">
              <ProgressBar done={doneCount} total={items.length} />
            </div>
            <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1">
              {byCategory.map((c) => (
                <div
                  key={c.key}
                  className="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400"
                >
                  <span className={`h-2 w-2 rounded-full ${c.dot}`} />
                  {c.label}
                  <span className="text-gray-400 dark:text-gray-500">
                    {c.done}/{c.total}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Resolution track record by year */}
      {!loading && byYear.length > 0 && (
        <div className="mt-3 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <div className="text-sm font-semibold text-gray-900 dark:text-gray-100">
            Resolutions by year
          </div>
          <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            How many of each year's resolutions you achieved.
          </p>
          <div className="mt-3 space-y-2.5">
            {byYear.map((y) => (
              <div key={y.year} className="flex items-center gap-3">
                <div className="w-10 shrink-0 text-sm font-medium text-gray-700 dark:text-gray-300">
                  {y.year}
                </div>
                <div
                  className="h-5 flex-1 overflow-hidden rounded-md bg-gray-100 dark:bg-gray-700"
                  style={{ maxWidth: `${(y.total / maxYearTotal) * 100}%` }}
                >
                  <div
                    className="flex h-full items-center rounded-md bg-indigo-600 pl-2 text-xs font-medium text-white transition-all dark:bg-indigo-500"
                    style={{ width: `${y.total ? (y.achieved / y.total) * 100 : 0}%` }}
                  />
                </div>
                <div className="shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400">
                  {y.achieved}/{y.total} achieved
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* By category breakdown */}
      {!loading && byCategory.length > 0 && (
        <div className="mt-3 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <div className="text-sm font-semibold text-gray-900 dark:text-gray-100">By category</div>
          <div className="mt-3 space-y-3">
            {byCategory.map((c) => (
              <div key={c.key}>
                <div className="mb-1 flex items-center justify-between text-xs">
                  <span className="flex items-center gap-1.5 font-medium text-gray-700 dark:text-gray-300">
                    <span className={`h-2 w-2 rounded-full ${c.dot}`} />
                    {c.label}
                  </span>
                  <span className="text-gray-400 dark:text-gray-500">
                    {c.done}/{c.total} done
                  </span>
                </div>
                <ProgressBar done={c.done} total={c.total} />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Add form */}
      <form
        onSubmit={add}
        className="mt-4 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4"
      >
        <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          Add something
        </label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <input
            value={newTitle}
            onChange={(e) => setNewTitle(e.target.value)}
            placeholder="e.g. Take a swimming course"
            className={inputClass}
          />
          <select
            value={newCategory}
            onChange={(e) => setNewCategory(e.target.value as BucketCategory)}
            className={`${inputClass} sm:w-56`}
          >
            {CATEGORIES.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
          <button
            type="submit"
            disabled={adding || !newTitle.trim()}
            className="shrink-0 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600 disabled:opacity-50"
          >
            {adding ? 'Adding…' : '+ Add'}
          </button>
        </div>
        <textarea
          value={newDescription}
          onChange={(e) => setNewDescription(e.target.value)}
          placeholder="Add a description (optional)"
          rows={2}
          className={`${inputClass} mt-2 resize-none`}
        />
        <input
          value={newNote}
          onChange={(e) => setNewNote(e.target.value)}
          placeholder="Add a note (optional)"
          className={`${inputClass} mt-2`}
        />
        <label className="mt-2 flex cursor-pointer items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input
            type="checkbox"
            checked={newResolution}
            onChange={(e) => setNewResolution(e.target.checked)}
            className="h-4 w-4 rounded border-gray-300 text-indigo-600 focus:ring-indigo-500 dark:border-gray-600 dark:bg-gray-900"
          />
          Make it a {CURRENT_YEAR} resolution
        </label>
      </form>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {/* Filters */}
      {!loading && items.length > 0 && (
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <FilterChip active={filterCat === 'all'} onClick={() => setFilterCat('all')}>
            All
          </FilterChip>
          {CATEGORIES.filter((c) => items.some((g) => g.category === c.key)).map((c) => (
            <FilterChip
              key={c.key}
              active={filterCat === c.key}
              onClick={() => setFilterCat(c.key)}
            >
              <span className={`h-2 w-2 rounded-full ${c.dot}`} />
              {c.label}
            </FilterChip>
          ))}
          <div className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
          <FilterChip active={resolutionsOnly} onClick={() => setResolutionsOnly((v) => !v)}>
            ★ Resolutions only
          </FilterChip>
        </div>
      )}

      {loading ? (
        <div className="mt-5 space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="flex items-start gap-3 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4"
            >
              <Skeleton className="h-6 w-6 shrink-0 rounded-full" />
              <div className="min-w-0 flex-1">
                <Skeleton className="h-3.5 w-48 max-w-full" />
                <Skeleton className="mt-2 h-3 w-64 max-w-full" />
              </div>
            </div>
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">
          Your bucket list is empty. Add something you want to do above.
        </p>
      ) : filtered.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">
          Nothing matches this filter.
        </p>
      ) : groups ? (
        <div className="mt-5 space-y-6">
          {groups.map((grp) => (
            <div key={grp.meta.key}>
              <div className="mb-2 flex items-center gap-2">
                <span className={`h-2.5 w-2.5 rounded-full ${grp.meta.dot}`} />
                <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
                  {grp.meta.label}
                </h2>
                <span className="text-xs text-gray-400 dark:text-gray-500">
                  {grp.items.filter((g) => g.done).length}/{grp.items.length}
                </span>
              </div>
              <div className="space-y-2">
                {grp.items.map((g) =>
                  editingId === g.id ? (
                    <EditRow
                      key={g.id}
                      item={g}
                      onCancel={() => setEditingId(null)}
                      onSave={(payload) => saveEdit(g.id, payload)}
                    />
                  ) : (
                    <ItemRow
                      key={g.id}
                      item={g}
                      busy={busyId === g.id}
                      showCategory={false}
                      onToggleDone={() => toggleDone(g)}
                      onToggleResolution={() => toggleResolution(g)}
                      onEdit={() => setEditingId(g.id)}
                      onDelete={() => remove(g)}
                    />
                  ),
                )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-5 space-y-2">
          {filtered.map((g) =>
            editingId === g.id ? (
              <EditRow
                key={g.id}
                item={g}
                onCancel={() => setEditingId(null)}
                onSave={(payload) => saveEdit(g.id, payload)}
              />
            ) : (
              <ItemRow
                key={g.id}
                item={g}
                busy={busyId === g.id}
                showCategory
                onToggleDone={() => toggleDone(g)}
                onToggleResolution={() => toggleResolution(g)}
                onEdit={() => setEditingId(g.id)}
                onDelete={() => remove(g)}
              />
            ),
          )}
        </div>
      )}
    </div>
  );
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition ${
        active
          ? 'border-indigo-600 bg-indigo-600 text-white dark:border-indigo-500 dark:bg-indigo-500'
          : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'
      }`}
    >
      {children}
    </button>
  );
}

function ItemRow({
  item: g,
  busy,
  showCategory,
  onToggleDone,
  onToggleResolution,
  onEdit,
  onDelete,
}: {
  item: BucketItem;
  busy: boolean;
  showCategory: boolean;
  onToggleDone: () => void;
  onToggleResolution: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const isPastResolution = g.resolution_year != null && g.resolution_year !== CURRENT_YEAR;
  return (
    <div className="flex items-start gap-3 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <CheckCircle done={g.done} busy={busy} onClick={onToggleDone} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span
            className={`text-sm font-semibold ${
              g.done
                ? 'text-gray-400 dark:text-gray-500 line-through'
                : 'text-gray-900 dark:text-gray-100'
            }`}
          >
            {g.title}
          </span>
          {showCategory && <CategoryBadge category={g.category} />}
          {isPastResolution && (
            <span className="inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
              ★ {g.resolution_year} resolution
            </span>
          )}
        </div>
        {g.description && (
          <p
            className={`mt-0.5 text-sm ${
              g.done ? 'text-gray-300 dark:text-gray-600' : 'text-gray-500 dark:text-gray-400'
            }`}
          >
            {g.description}
          </p>
        )}
        {g.note && (
          <p
            className={`mt-1 text-xs ${
              g.done ? 'text-gray-300 dark:text-gray-600' : 'text-gray-400 dark:text-gray-500'
            }`}
          >
            {g.note}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <ResolutionStar
          active={g.resolution_year === CURRENT_YEAR}
          busy={busy}
          onClick={onToggleResolution}
        />
        <button
          type="button"
          onClick={onEdit}
          className="text-sm font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
        >
          Edit
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={onDelete}
          className="text-sm font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300 disabled:opacity-50"
        >
          Delete
        </button>
      </div>
    </div>
  );
}

function EditRow({
  item: g,
  onSave,
  onCancel,
}: {
  item: BucketItem;
  onSave: (payload: {
    title: string;
    description: string;
    note: string;
    category: BucketCategory;
  }) => Promise<void>;
  onCancel: () => void;
}) {
  const [title, setTitle] = useState(g.title);
  const [category, setCategory] = useState<BucketCategory>(g.category);
  const [description, setDescription] = useState(g.description);
  const [note, setNote] = useState(g.note);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    setSaving(true);
    setError('');
    try {
      await onSave({ title, description, note, category });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={submit}
      className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4"
    >
      <div className="flex flex-col gap-2 sm:flex-row">
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Title"
          className={inputClass}
          autoFocus
        />
        <select
          value={category}
          onChange={(e) => setCategory(e.target.value as BucketCategory)}
          className={`${inputClass} sm:w-56`}
        >
          {CATEGORIES.map((c) => (
            <option key={c.key} value={c.key}>
              {c.label}
            </option>
          ))}
        </select>
      </div>
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder="Description (optional)"
        rows={2}
        className={`${inputClass} mt-2 resize-none`}
      />
      <input
        value={note}
        onChange={(e) => setNote(e.target.value)}
        placeholder="Note (optional)"
        className={`${inputClass} mt-2`}
      />
      {error && <p className="mt-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
      <div className="mt-3 flex items-center gap-3">
        <button
          type="submit"
          disabled={saving || !title.trim()}
          className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600 disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-300 transition hover:bg-gray-100 dark:hover:bg-gray-700"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
