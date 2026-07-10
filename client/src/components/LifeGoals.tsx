import { useState, useEffect } from 'react';
import {
  listLifeGoals,
  createLifeGoal,
  updateLifeGoal,
  setLifeGoalDone,
  deleteLifeGoal,
} from '../api/client';
import type { LifeGoal } from '../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 dark:border-gray-700 dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 outline-none transition focus:border-indigo-500 dark:focus:border-indigo-400 focus:ring-2 focus:ring-indigo-200 dark:focus:ring-indigo-500/30';

// A circular check control used for each life-list item.
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

export function LifeGoals() {
  const [goals, setGoals] = useState<LifeGoal[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editingId, setEditingId] = useState<number | null>(null);

  // New-item form state.
  const [newTitle, setNewTitle] = useState('');
  const [newNote, setNewNote] = useState('');
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    let active = true;
    listLifeGoals()
      .then((gs) => active && setGoals(gs))
      .catch(
        (e) => active && setError(e instanceof Error ? e.message : 'Failed to load life goals'),
      )
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => setGoals(await listLifeGoals());

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    const title = newTitle.trim();
    if (!title) return;
    setAdding(true);
    setError('');
    try {
      await createLifeGoal({ title, note: newNote.trim() });
      setNewTitle('');
      setNewNote('');
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add');
    } finally {
      setAdding(false);
    }
  };

  const toggle = async (g: LifeGoal) => {
    setBusyId(g.id);
    setError('');
    try {
      await setLifeGoalDone(g.id, !g.done);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (g: LifeGoal) => {
    setBusyId(g.id);
    setError('');
    try {
      await deleteLifeGoal(g.id);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete');
    } finally {
      setBusyId(null);
    }
  };

  const saveEdit = async (id: number, title: string, note: string) => {
    await updateLifeGoal(id, { title: title.trim(), note: note.trim() });
    await reload();
    setEditingId(null);
  };

  const doneCount = goals.filter((g) => g.done).length;
  const pct = goals.length ? Math.round((doneCount / goals.length) * 100) : 0;

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
          Life Goals
        </h1>
        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
          A checklist of things you want to do in life. Add them here or just tell the assistant —
          then check them off as you go.
        </p>
      </div>

      {/* Progress */}
      {goals.length > 0 && (
        <div className="mt-4 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <div className="flex items-center justify-between text-sm">
            <span className="font-semibold text-gray-900 dark:text-gray-100">
              {doneCount} of {goals.length} done
            </span>
            <span className="text-gray-500 dark:text-gray-400">{pct}%</span>
          </div>
          <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-gray-700">
            <div
              className="h-full rounded-full bg-indigo-600 dark:bg-indigo-500 transition-all"
              style={{ width: `${pct}%` }}
            />
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
          <button
            type="submit"
            disabled={adding || !newTitle.trim()}
            className="shrink-0 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600 disabled:opacity-50"
          >
            {adding ? 'Adding…' : '+ Add'}
          </button>
        </div>
        <input
          value={newNote}
          onChange={(e) => setNewNote(e.target.value)}
          placeholder="Add a note (optional)"
          className={`${inputClass} mt-2`}
        />
      </form>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">Loading…</p>
      ) : goals.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">
          Your life list is empty. Add something you want to do above.
        </p>
      ) : (
        <div className="mt-5 space-y-2">
          {goals.map((g) =>
            editingId === g.id ? (
              <EditRow
                key={g.id}
                goal={g}
                onCancel={() => setEditingId(null)}
                onSave={(title, note) => saveEdit(g.id, title, note)}
              />
            ) : (
              <div
                key={g.id}
                className="flex items-start gap-3 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4"
              >
                <CheckCircle done={g.done} busy={busyId === g.id} onClick={() => toggle(g)} />
                <div className="min-w-0 flex-1">
                  <div
                    className={`text-sm font-semibold ${
                      g.done
                        ? 'text-gray-400 dark:text-gray-500 line-through'
                        : 'text-gray-900 dark:text-gray-100'
                    }`}
                  >
                    {g.title}
                  </div>
                  {g.note && (
                    <p
                      className={`mt-0.5 text-sm ${
                        g.done
                          ? 'text-gray-300 dark:text-gray-600'
                          : 'text-gray-500 dark:text-gray-400'
                      }`}
                    >
                      {g.note}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => setEditingId(g.id)}
                    className="text-sm font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    disabled={busyId === g.id}
                    onClick={() => remove(g)}
                    className="text-sm font-medium text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300 disabled:opacity-50"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ),
          )}
        </div>
      )}
    </div>
  );
}

function EditRow({
  goal,
  onSave,
  onCancel,
}: {
  goal: LifeGoal;
  onSave: (title: string, note: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [title, setTitle] = useState(goal.title);
  const [note, setNote] = useState(goal.note);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    setSaving(true);
    setError('');
    try {
      await onSave(title, note);
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
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Title"
        className={inputClass}
        autoFocus
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
