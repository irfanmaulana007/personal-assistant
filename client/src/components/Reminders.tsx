import { useState, useEffect } from 'react';
import {
  listReminders,
  createReminder,
  updateReminder,
  setReminderEnabled,
  deleteReminder,
  getRemindersConfig,
  setRemindersConfig,
} from '../api/client';
import type { Reminder, ReminderPayload, RepeatMode } from '../types';
import { Toggle } from './ui/Toggle';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

const MODES: { value: RepeatMode; label: string }[] = [
  { value: 'once', label: 'Once' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'monthly', label: 'Monthly' },
];

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function emptyForm(): ReminderPayload {
  return {
    title: '',
    repeat_mode: 'daily',
    times: ['09:00'],
    weekdays: [],
    day_of_month: 1,
    once_date: todayISO(),
    enabled: true,
  };
}

function formatDate(iso: string): string {
  const d = new Date(iso + 'T00:00:00');
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

function summarize(r: Reminder): string {
  const times = r.times.join(', ');
  switch (r.repeat_mode) {
    case 'daily':
      return `Every day · ${times}`;
    case 'weekly': {
      const days = [...r.weekdays].sort((a, b) => a - b).map((d) => WEEKDAYS[d]);
      return `Weekly · ${days.join(', ')} · ${times}`;
    }
    case 'monthly':
      return `Monthly · day ${r.day_of_month} · ${times}`;
    default:
      return `Once · ${formatDate(r.once_date)} · ${times}`;
  }
}

export function Reminders({ isAdmin }: { isAdmin: boolean }) {
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [globalEnabled, setGlobalEnabled] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editing, setEditing] = useState<{ id: number | null; form: ReminderPayload } | null>(null);

  useEffect(() => {
    let active = true;
    Promise.all([listReminders(), getRemindersConfig()])
      .then(([rs, cfg]) => {
        if (!active) return;
        setReminders(rs);
        setGlobalEnabled(cfg.enabled);
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load reminders'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => setReminders(await listReminders());

  const toggleGlobal = async () => {
    setError('');
    try {
      const cfg = await setRemindersConfig(!globalEnabled);
      setGlobalEnabled(cfg.enabled);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update setting');
    }
  };

  const toggleReminder = async (r: Reminder) => {
    setBusyId(r.id);
    setError('');
    try {
      await setReminderEnabled(r.id, !r.enabled);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update reminder');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (r: Reminder) => {
    setBusyId(r.id);
    setError('');
    try {
      await deleteReminder(r.id);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete reminder');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900">Reminders</h1>
          <p className="mt-0.5 text-sm text-gray-500">
            Schedule reminders delivered over WhatsApp. Set them to repeat and add one or more
            times.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-gray-600">All reminders</span>
          <Toggle on={globalEnabled} disabled={!isAdmin} onClick={toggleGlobal} />
        </div>
      </div>

      {!isAdmin && (
        <p className="mt-1 text-xs text-gray-400">
          Only an admin can turn all reminders on or off.
        </p>
      )}

      {!globalEnabled && (
        <div className="mt-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          Reminders are paused. Nothing will be delivered until they’re turned back on.
        </div>
      )}

      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

      <div className="mt-5">
        {editing ? (
          <ReminderForm
            initial={editing.form}
            editing={editing.id !== null}
            onCancel={() => setEditing(null)}
            onSave={async (payload) => {
              if (editing.id === null) await createReminder(payload);
              else await updateReminder(editing.id, payload);
              await reload();
              setEditing(null);
            }}
          />
        ) : (
          <button
            type="button"
            onClick={() => setEditing({ id: null, form: emptyForm() })}
            className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700"
          >
            + New reminder
          </button>
        )}
      </div>

      {loading ? (
        <p className="mt-6 text-sm text-gray-500">Loading…</p>
      ) : reminders.length === 0 && !editing ? (
        <p className="mt-6 text-sm text-gray-500">No reminders yet.</p>
      ) : (
        <div className="mt-5 space-y-2">
          {reminders.map((r) => (
            <div
              key={r.id}
              className="flex items-start gap-4 rounded-2xl border border-gray-200 bg-white p-4"
            >
              <div className="min-w-0 flex-1">
                <div className="text-sm font-semibold text-gray-900">{r.title}</div>
                <p className="mt-0.5 text-sm text-gray-500">{summarize(r)}</p>
              </div>
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => setEditing({ id: r.id, form: toPayload(r) })}
                  className="text-sm font-medium text-indigo-600 hover:text-indigo-700"
                >
                  Edit
                </button>
                <button
                  type="button"
                  disabled={busyId === r.id}
                  onClick={() => remove(r)}
                  className="text-sm font-medium text-red-600 hover:text-red-700 disabled:opacity-50"
                >
                  Delete
                </button>
                <Toggle on={r.enabled} busy={busyId === r.id} onClick={() => toggleReminder(r)} />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function toPayload(r: Reminder): ReminderPayload {
  return {
    title: r.title,
    repeat_mode: r.repeat_mode,
    times: r.times,
    weekdays: r.weekdays,
    day_of_month: r.day_of_month,
    once_date: r.once_date,
    enabled: r.enabled,
  };
}

function ReminderForm({
  initial,
  editing,
  onSave,
  onCancel,
}: {
  initial: ReminderPayload;
  editing: boolean;
  onSave: (payload: ReminderPayload) => Promise<void>;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<ReminderPayload>(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const set = <K extends keyof ReminderPayload>(k: K, v: ReminderPayload[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const setMode = (mode: RepeatMode) => {
    setForm((f) => ({
      ...f,
      repeat_mode: mode,
      weekdays: mode === 'weekly' && f.weekdays.length === 0 ? [1] : f.weekdays,
      day_of_month: mode === 'monthly' && !f.day_of_month ? 1 : f.day_of_month,
      once_date: mode === 'once' && !f.once_date ? todayISO() : f.once_date,
    }));
  };

  const toggleWeekday = (d: number) =>
    setForm((f) => ({
      ...f,
      weekdays: f.weekdays.includes(d) ? f.weekdays.filter((x) => x !== d) : [...f.weekdays, d],
    }));

  const setTime = (i: number, v: string) =>
    setForm((f) => ({ ...f, times: f.times.map((t, idx) => (idx === i ? v : t)) }));
  const addTime = () => setForm((f) => ({ ...f, times: [...f.times, '09:00'] }));
  const removeTime = (i: number) =>
    setForm((f) => ({ ...f, times: f.times.filter((_, idx) => idx !== i) }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError('');
    try {
      await onSave(form);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <form onSubmit={submit} className="rounded-2xl border border-gray-200 bg-white p-5">
      <h2 className="mb-4 text-base font-semibold text-gray-900">
        {editing ? 'Edit reminder' : 'New reminder'}
      </h2>

      <div className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Title</label>
          <input
            value={form.title}
            onChange={(e) => set('title', e.target.value)}
            placeholder="e.g. Take vitamins"
            className={inputClass}
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Repeat</label>
          <div className="flex flex-wrap gap-2">
            {MODES.map((m) => (
              <button
                key={m.value}
                type="button"
                onClick={() => setMode(m.value)}
                className={`rounded-xl border px-3 py-1.5 text-sm transition ${
                  form.repeat_mode === m.value
                    ? 'border-indigo-600 bg-indigo-50 font-medium text-indigo-700'
                    : 'border-gray-200 text-gray-600 hover:bg-gray-50'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>
        </div>

        {form.repeat_mode === 'once' && (
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">Date</label>
            <input
              type="date"
              value={form.once_date}
              onChange={(e) => set('once_date', e.target.value)}
              className={inputClass}
            />
          </div>
        )}

        {form.repeat_mode === 'weekly' && (
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">On days</label>
            <div className="flex flex-wrap gap-2">
              {WEEKDAYS.map((label, d) => (
                <button
                  key={d}
                  type="button"
                  onClick={() => toggleWeekday(d)}
                  className={`h-9 w-11 rounded-xl border text-sm transition ${
                    form.weekdays.includes(d)
                      ? 'border-indigo-600 bg-indigo-50 font-medium text-indigo-700'
                      : 'border-gray-200 text-gray-600 hover:bg-gray-50'
                  }`}
                >
                  {label.slice(0, 2)}
                </button>
              ))}
            </div>
          </div>
        )}

        {form.repeat_mode === 'monthly' && (
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">Day of month</label>
            <input
              type="number"
              min={1}
              max={31}
              value={form.day_of_month}
              onChange={(e) => set('day_of_month', Number(e.target.value))}
              className={`${inputClass} max-w-[8rem]`}
            />
            <p className="mt-1 text-xs text-gray-400">
              A day past the month’s length (e.g. 31 in February) fires on the last day.
            </p>
          </div>
        )}

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Times</label>
          <div className="space-y-2">
            {form.times.map((t, i) => (
              <div key={i} className="flex items-center gap-2">
                <input
                  type="time"
                  value={t}
                  onChange={(e) => setTime(i, e.target.value)}
                  className={`${inputClass} max-w-[10rem]`}
                />
                {form.times.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeTime(i)}
                    className="text-sm font-medium text-gray-400 hover:text-red-600"
                    aria-label="Remove time"
                  >
                    Remove
                  </button>
                )}
              </div>
            ))}
          </div>
          <button
            type="button"
            onClick={addTime}
            className="mt-2 text-sm font-medium text-indigo-600 hover:text-indigo-700"
          >
            + Add time
          </button>
        </div>

        <div className="flex items-center gap-2">
          <Toggle on={form.enabled} onClick={() => set('enabled', !form.enabled)} />
          <span className="text-sm text-gray-600">Active</span>
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

      <div className="mt-5 flex items-center gap-3">
        <button
          type="submit"
          disabled={saving}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-xl px-4 py-2.5 text-sm font-medium text-gray-600 transition hover:bg-gray-100"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
