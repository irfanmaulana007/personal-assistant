import { useState, useEffect, useMemo } from 'react';
import {
  listWishItems,
  createWishItem,
  updateWishItem,
  setWishItemDone,
  deleteWishItem,
} from '../api/client';
import type { WishItem, WishPriority } from '../types';
import { Skeleton } from './ui/Skeleton';
import { Modal } from './ui/Modal';

const inputClass =
  'w-full rounded-xl border border-gray-200 dark:border-gray-700 dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 outline-none transition focus:border-indigo-500 dark:focus:border-indigo-400 focus:ring-2 focus:ring-indigo-200 dark:focus:ring-indigo-500/30';

// Whole-number amount rendered with thousands separators, prefixed "Rp".
function formatPrice(v: number): string {
  return 'Rp' + Math.round(v).toLocaleString('id-ID');
}

// "YYYY-MM" → "September 2026"; '' → "No month yet".
function monthLabel(m: string): string {
  if (!m) return 'No month yet';
  const [y, mo] = m.split('-').map(Number);
  if (!y || !mo) return m;
  return new Date(y, mo - 1, 1).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'long',
  });
}

interface PriorityMeta {
  key: WishPriority;
  label: string;
  dot: string;
  badge: string;
}

// Order here drives filter-chip and sort order (high → low).
const PRIORITIES: PriorityMeta[] = [
  {
    key: 'high',
    label: 'High',
    dot: 'bg-rose-500',
    badge: 'bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300',
  },
  {
    key: 'medium',
    label: 'Medium',
    dot: 'bg-amber-500',
    badge: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  },
  {
    key: 'low',
    label: 'Low',
    dot: 'bg-gray-400 dark:bg-gray-500',
    badge: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
  },
];

const PRIORITY_BY_KEY: Record<WishPriority, PriorityMeta> = PRIORITIES.reduce(
  (acc, p) => {
    acc[p.key] = p;
    return acc;
  },
  {} as Record<WishPriority, PriorityMeta>,
);

const PRIORITY_RANK: Record<WishPriority, number> = { high: 0, medium: 1, low: 2 };

function priorityMeta(key: WishPriority): PriorityMeta {
  return PRIORITY_BY_KEY[key] ?? PRIORITY_BY_KEY.medium;
}

// A circular check control marking an item as bought.
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
      aria-label={done ? 'Mark as not bought' : 'Mark as bought'}
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

function PriorityBadge({ priority }: { priority: WishPriority }) {
  const m = priorityMeta(priority);
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${m.badge}`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${m.dot}`} />
      {m.label}
    </span>
  );
}

interface WishFormValues {
  name: string;
  estimated_price: string; // kept as string for the input; parsed on submit
  buy_month: string; // "YYYY-MM" or ''
  priority: WishPriority;
  link: string;
  note: string;
}

export function Wishlist() {
  const [items, setItems] = useState<WishItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [showAdd, setShowAdd] = useState(false);

  // Filters.
  const [filterPriority, setFilterPriority] = useState<WishPriority | 'all'>('all');
  const [hideBought, setHideBought] = useState(false);

  useEffect(() => {
    let active = true;
    listWishItems()
      .then((ws) => active && setItems(ws))
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load wishlist'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => setItems(await listWishItems());

  const toPayload = (v: WishFormValues) => ({
    name: v.name.trim(),
    estimated_price: Math.max(0, Math.round(Number(v.estimated_price) || 0)),
    buy_month: v.buy_month,
    priority: v.priority,
    link: v.link.trim(),
    note: v.note.trim(),
  });

  const add = async (v: WishFormValues) => {
    await createWishItem(toPayload(v));
    await reload();
    setShowAdd(false);
  };

  const saveEdit = async (id: number, v: WishFormValues) => {
    await updateWishItem(id, toPayload(v));
    await reload();
    setEditingId(null);
  };

  const toggleDone = async (w: WishItem) => {
    setBusyId(w.id);
    setError('');
    try {
      await setWishItemDone(w.id, !w.done);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (w: WishItem) => {
    setBusyId(w.id);
    setError('');
    try {
      await deleteWishItem(w.id);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete');
    } finally {
      setBusyId(null);
    }
  };

  const editingItem = editingId != null ? items.find((w) => w.id === editingId) : undefined;

  // --- Derived stats ---------------------------------------------------------

  const boughtCount = items.filter((w) => w.done).length;
  const pendingTotal = items.filter((w) => !w.done).reduce((s, w) => s + w.estimated_price, 0);
  const grandTotal = items.reduce((s, w) => s + w.estimated_price, 0);

  // --- Filtering & grouping by target month ----------------------------------

  const filtered = useMemo(() => {
    return items.filter((w) => {
      if (hideBought && w.done) return false;
      if (filterPriority !== 'all' && w.priority !== filterPriority) return false;
      return true;
    });
  }, [items, filterPriority, hideBought]);

  // Group by buy_month; dated months sorted chronologically, "No month yet"
  // pushed to the end. Within a group: not-bought first, then priority, then name.
  const groups = useMemo(() => {
    const byMonth = new Map<string, WishItem[]>();
    for (const w of filtered) {
      const arr = byMonth.get(w.buy_month) ?? [];
      arr.push(w);
      byMonth.set(w.buy_month, arr);
    }
    const keys = [...byMonth.keys()].sort((a, b) => {
      if (a === '') return 1;
      if (b === '') return -1;
      return a.localeCompare(b);
    });
    return keys.map((month) => {
      const list = (byMonth.get(month) ?? []).slice().sort((a, b) => {
        if (a.done !== b.done) return a.done ? 1 : -1;
        if (PRIORITY_RANK[a.priority] !== PRIORITY_RANK[b.priority]) {
          return PRIORITY_RANK[a.priority] - PRIORITY_RANK[b.priority];
        }
        return a.name.localeCompare(b.name);
      });
      const pending = list.filter((w) => !w.done).reduce((s, w) => s + w.estimated_price, 0);
      const total = list.reduce((s, w) => s + w.estimated_price, 0);
      return { month, items: list, pending, total };
    });
  }, [filtered]);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Wishlist
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Everything you plan to buy, grouped by the month you want to buy it — so you can plan
            your spending and check out after payroll.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setShowAdd(true)}
          className="shrink-0 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
        >
          + Add item
        </button>
      </div>

      {/* Overview: still-to-buy total + bought progress */}
      {!loading && items.length > 0 && (
        <div className="mt-4 grid gap-3 sm:grid-cols-3">
          <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
            <div className="text-sm font-medium text-gray-500 dark:text-gray-400">Still to buy</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {formatPrice(pendingTotal)}
            </div>
            <div className="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
              {items.length - boughtCount} item{items.length - boughtCount === 1 ? '' : 's'} pending
            </div>
          </div>
          <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
            <div className="text-sm font-medium text-gray-500 dark:text-gray-400">
              Total planned
            </div>
            <div className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {formatPrice(grandTotal)}
            </div>
            <div className="mt-0.5 text-xs text-gray-400 dark:text-gray-500">
              across {items.length} item{items.length === 1 ? '' : 's'}
            </div>
          </div>
          <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
            <div className="text-sm font-medium text-gray-500 dark:text-gray-400">Bought</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {boughtCount}
              <span className="text-base font-normal text-gray-400 dark:text-gray-500">
                {' '}
                / {items.length}
              </span>
            </div>
            <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-gray-700">
              <div
                className="h-full rounded-full bg-indigo-600 transition-all dark:bg-indigo-500"
                style={{ width: `${items.length ? (boughtCount / items.length) * 100 : 0}%` }}
              />
            </div>
          </div>
        </div>
      )}

      {/* Add / edit modals */}
      <Modal open={showAdd} onClose={() => setShowAdd(false)} title="Add to wishlist">
        {showAdd && (
          <WishItemForm
            initial={{
              name: '',
              estimated_price: '',
              buy_month: '',
              priority: 'medium',
              link: '',
              note: '',
            }}
            submitLabel="Add"
            onSubmit={add}
            onCancel={() => setShowAdd(false)}
          />
        )}
      </Modal>

      <Modal open={editingItem != null} onClose={() => setEditingId(null)} title="Edit item">
        {editingItem && (
          <WishItemForm
            initial={{
              name: editingItem.name,
              estimated_price: editingItem.estimated_price
                ? String(editingItem.estimated_price)
                : '',
              buy_month: editingItem.buy_month,
              priority: editingItem.priority,
              link: editingItem.link,
              note: editingItem.note,
            }}
            submitLabel="Save"
            onSubmit={(v) => saveEdit(editingItem.id, v)}
            onCancel={() => setEditingId(null)}
          />
        )}
      </Modal>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {/* Filters */}
      {!loading && items.length > 0 && (
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <FilterChip active={filterPriority === 'all'} onClick={() => setFilterPriority('all')}>
            All
          </FilterChip>
          {PRIORITIES.filter((p) => items.some((w) => w.priority === p.key)).map((p) => (
            <FilterChip
              key={p.key}
              active={filterPriority === p.key}
              onClick={() => setFilterPriority(p.key)}
            >
              <span className={`h-2 w-2 rounded-full ${p.dot}`} />
              {p.label}
            </FilterChip>
          ))}
          <div className="mx-1 h-5 w-px bg-gray-200 dark:bg-gray-700" />
          <FilterChip active={hideBought} onClick={() => setHideBought((v) => !v)}>
            Hide bought
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
          Your wishlist is empty. Add something you want to buy above.
        </p>
      ) : filtered.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">
          Nothing matches this filter.
        </p>
      ) : (
        <div className="mt-5 space-y-6">
          {groups.map((grp) => (
            <div key={grp.month || 'none'}>
              <div className="mb-2 flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <svg
                    className="h-4 w-4 text-gray-400 dark:text-gray-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
                    />
                  </svg>
                  <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
                    {monthLabel(grp.month)}
                  </h2>
                  <span className="text-xs text-gray-400 dark:text-gray-500">
                    {grp.items.length} item{grp.items.length === 1 ? '' : 's'}
                  </span>
                </div>
                <span className="shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400">
                  {grp.pending < grp.total ? (
                    <>
                      {formatPrice(grp.pending)}{' '}
                      <span className="text-gray-400 dark:text-gray-500">
                        / {formatPrice(grp.total)}
                      </span>
                    </>
                  ) : (
                    formatPrice(grp.total)
                  )}
                </span>
              </div>
              <div className="space-y-2">
                {grp.items.map((w) => (
                  <ItemRow
                    key={w.id}
                    item={w}
                    busy={busyId === w.id}
                    onToggleDone={() => toggleDone(w)}
                    onEdit={() => setEditingId(w.id)}
                    onDelete={() => remove(w)}
                  />
                ))}
              </div>
            </div>
          ))}
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
  item: w,
  busy,
  onToggleDone,
  onEdit,
  onDelete,
}: {
  item: WishItem;
  busy: boolean;
  onToggleDone: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="flex items-start gap-3 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <CheckCircle done={w.done} busy={busy} onClick={onToggleDone} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span
            className={`text-sm font-semibold ${
              w.done
                ? 'text-gray-400 dark:text-gray-500 line-through'
                : 'text-gray-900 dark:text-gray-100'
            }`}
          >
            {w.name}
          </span>
          {w.estimated_price > 0 && (
            <span
              className={`text-sm font-medium ${
                w.done ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-300'
              }`}
            >
              {formatPrice(w.estimated_price)}
            </span>
          )}
          <PriorityBadge priority={w.priority} />
        </div>
        {w.note && (
          <p
            className={`mt-1 text-xs ${
              w.done ? 'text-gray-300 dark:text-gray-600' : 'text-gray-400 dark:text-gray-500'
            }`}
          >
            {w.note}
          </p>
        )}
        {w.link && (
          <a
            href={w.link}
            target="_blank"
            rel="noopener noreferrer"
            className="mt-1 inline-flex max-w-full items-center gap-1 truncate text-xs font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
          >
            <svg
              className="h-3.5 w-3.5 shrink-0"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M13.828 10.172a4 4 0 010 5.656l-3 3a4 4 0 01-5.656-5.656l1.5-1.5m6.828-6.828l3-3a4 4 0 015.656 5.656l-1.5 1.5m-6.828 0a4 4 0 015.656 0"
              />
            </svg>
            <span className="truncate">{linkLabel(w.link)}</span>
          </a>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
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

// Show a friendly hostname for a link instead of the full URL.
function linkLabel(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return url;
  }
}

// Shared form body for both adding and editing a wishlist item. Rendered inside
// a Modal, so it carries no card chrome of its own.
function WishItemForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
}: {
  initial: WishFormValues;
  submitLabel: string;
  onSubmit: (payload: WishFormValues) => Promise<void>;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<WishFormValues>(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const set = <K extends keyof WishFormValues>(key: K, value: WishFormValues[K]) =>
    setForm((f) => ({ ...f, [key]: value }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) return;
    setSaving(true);
    setError('');
    try {
      await onSubmit(form);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
      setSaving(false);
    }
  };

  return (
    <form onSubmit={submit}>
      <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
        Item name
      </label>
      <input
        value={form.name}
        onChange={(e) => set('name', e.target.value)}
        placeholder="e.g. Rain shower head"
        className={inputClass}
        autoFocus
      />

      <div className="mt-3 flex flex-col gap-3 sm:flex-row">
        <div className="flex-1">
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Estimated price
          </label>
          <div className="relative">
            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-gray-400 dark:text-gray-500">
              Rp
            </span>
            <input
              type="number"
              min={0}
              step={1000}
              value={form.estimated_price}
              onChange={(e) => set('estimated_price', e.target.value)}
              placeholder="0"
              className={`${inputClass} pl-9`}
            />
          </div>
        </div>
        <div className="sm:w-44">
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Buy month
          </label>
          <input
            type="month"
            value={form.buy_month}
            onChange={(e) => set('buy_month', e.target.value)}
            className={inputClass}
          />
        </div>
      </div>

      <div className="mt-3">
        <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          Priority
        </label>
        <select
          value={form.priority}
          onChange={(e) => set('priority', e.target.value as WishPriority)}
          className={inputClass}
        >
          {PRIORITIES.map((p) => (
            <option key={p.key} value={p.key}>
              {p.label}
            </option>
          ))}
        </select>
      </div>

      <div className="mt-3">
        <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          Link <span className="font-normal text-gray-400 dark:text-gray-500">(optional)</span>
        </label>
        <input
          type="url"
          value={form.link}
          onChange={(e) => set('link', e.target.value)}
          placeholder="https://… reference or marketplace link"
          className={inputClass}
        />
      </div>

      <div className="mt-3">
        <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
          Note <span className="font-normal text-gray-400 dark:text-gray-500">(optional)</span>
        </label>
        <input
          value={form.note}
          onChange={(e) => set('note', e.target.value)}
          placeholder="e.g. white, 60cm"
          className={inputClass}
        />
      </div>

      {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
      <div className="mt-5 flex items-center gap-3">
        <button
          type="submit"
          disabled={saving || !form.name.trim()}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600 disabled:opacity-50"
        >
          {saving ? 'Saving…' : submitLabel}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 dark:text-gray-300 transition hover:bg-gray-100 dark:hover:bg-gray-700"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
