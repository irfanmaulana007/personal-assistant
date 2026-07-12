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
import type { Reminder, ReminderPayload, RepeatMode, RemindersConfig } from '../types';
import { Toggle } from './ui/Toggle';
import { SkeletonListRow } from './ui/Skeleton';
import { Modal } from './ui/Modal';
import { DatePicker } from './DatePicker';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:ring-indigo-500/30';

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
    event_at: '',
    offsets: [],
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
  const [config, setConfig] = useState<RemindersConfig>({
    enabled: true,
    default_time: '09:00',
  });
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
        setConfig(cfg);
      })
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load reminders'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const reload = async () => setReminders(await listReminders());

  const saveConfig = async (next: RemindersConfig) => {
    setError('');
    const prev = config;
    setConfig(next); // optimistic
    try {
      setConfig(await setRemindersConfig(next));
    } catch (e) {
      setConfig(prev);
      setError(e instanceof Error ? e.message : 'Failed to update setting');
    }
  };

  const toggleGlobal = () => saveConfig({ ...config, enabled: !config.enabled });
  const setDefaultTime = (t: string) => t && saveConfig({ ...config, default_time: t });

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
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Reminders
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            Schedule reminders delivered over WhatsApp. Set them to repeat and add one or more
            times.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setEditing({ id: null, form: emptyForm() })}
          className="shrink-0 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 dark:bg-indigo-500 dark:hover:bg-indigo-600"
        >
          + New reminder
        </button>
      </div>

      {/* Reminder settings — a section distinct from the reminder list. */}
      <div className="mt-4 divide-y divide-gray-100 rounded-2xl border border-gray-200 bg-white dark:divide-gray-800 dark:border-gray-700 dark:bg-gray-800">
        <div className="flex items-start justify-between gap-4 p-4">
          <div className="min-w-0">
            <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
              All reminders
            </div>
            <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
              Turn every reminder on or off at once.
            </p>
          </div>
          <Toggle on={config.enabled} disabled={!isAdmin} onClick={toggleGlobal} />
        </div>
        <div className="flex items-start justify-between gap-4 p-4">
          <div className="min-w-0">
            <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
              Default reminder time
            </div>
            <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
              Used when you create a reminder without saying a time.
            </p>
          </div>
          <input
            type="time"
            value={config.default_time}
            disabled={!isAdmin}
            onChange={(e) => setDefaultTime(e.target.value)}
            className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 disabled:opacity-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:ring-indigo-500/30"
          />
        </div>
        {!isAdmin && (
          <p className="px-4 py-2 text-xs text-gray-400 dark:text-gray-500">
            Only an admin can change these settings.
          </p>
        )}
      </div>

      {!config.enabled && (
        <div className="mt-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/15 dark:text-amber-300">
          Reminders are paused. Nothing will be delivered until they’re turned back on.
        </div>
      )}

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <Modal
        open={editing !== null}
        onClose={() => setEditing(null)}
        title={editing?.id != null ? 'Edit reminder' : 'New reminder'}
      >
        {editing && (
          <ReminderForm
            initial={editing.form}
            onCancel={() => setEditing(null)}
            onSave={async (payload) => {
              if (editing.id === null) await createReminder(payload);
              else await updateReminder(editing.id, payload);
              await reload();
              setEditing(null);
            }}
          />
        )}
      </Modal>

      {loading ? (
        <div className="mt-5 space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <SkeletonListRow key={i} trailingWidth="w-28" />
          ))}
        </div>
      ) : reminders.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">No reminders yet.</p>
      ) : (
        <div className="mt-5 space-y-2">
          {reminders.map((r) => (
            <div
              key={r.id}
              className="flex items-start gap-4 rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
            >
              <div className="min-w-0 flex-1">
                <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                  {r.title}
                </div>
                <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{summarize(r)}</p>
              </div>
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => setEditing({ id: r.id, form: toPayload(r) })}
                  className="text-sm font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
                >
                  Edit
                </button>
                <button
                  type="button"
                  disabled={busyId === r.id}
                  onClick={() => remove(r)}
                  className="text-sm font-medium text-red-600 hover:text-red-700 disabled:opacity-50 dark:text-red-400 dark:hover:text-red-300"
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
    event_at: r.event_at,
    offsets: r.offsets,
    enabled: r.enabled,
  };
}

function ReminderForm({
  initial,
  onSave,
  onCancel,
}: {
  initial: ReminderPayload;
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
    <form onSubmit={submit}>
      <div className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Title
          </label>
          <input
            value={form.title}
            onChange={(e) => set('title', e.target.value)}
            placeholder="e.g. Take vitamins"
            className={inputClass}
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Repeat
          </label>
          <div className="flex flex-wrap gap-2">
            {MODES.map((m) => (
              <button
                key={m.value}
                type="button"
                onClick={() => setMode(m.value)}
                className={`rounded-xl border px-3 py-1.5 text-sm transition ${
                  form.repeat_mode === m.value
                    ? 'border-indigo-600 bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
                    : 'border-gray-200 text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800/60'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>
        </div>

        {form.repeat_mode === 'once' && (
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Date
            </label>
            <DatePicker value={form.once_date} onChange={(v) => set('once_date', v)} />
          </div>
        )}

        {form.repeat_mode === 'weekly' && (
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              On days
            </label>
            <div className="flex flex-wrap gap-2">
              {WEEKDAYS.map((label, d) => (
                <button
                  key={d}
                  type="button"
                  onClick={() => toggleWeekday(d)}
                  className={`h-9 w-11 rounded-xl border text-sm transition ${
                    form.weekdays.includes(d)
                      ? 'border-indigo-600 bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300'
                      : 'border-gray-200 text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800/60'
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
            <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
              Day of month
            </label>
            <input
              type="number"
              min={1}
              max={31}
              value={form.day_of_month}
              onChange={(e) => set('day_of_month', Number(e.target.value))}
              className={`${inputClass} max-w-[8rem]`}
            />
            <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
              A day past the month’s length (e.g. 31 in February) fires on the last day.
            </p>
          </div>
        )}

        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Times
          </label>
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
                    className="text-sm font-medium text-gray-400 hover:text-red-600 dark:text-gray-500 dark:hover:text-red-400"
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
            className="mt-2 text-sm font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
          >
            + Add time
          </button>
        </div>

        <div className="flex items-center gap-2">
          <Toggle on={form.enabled} onClick={() => set('enabled', !form.enabled)} />
          <span className="text-sm text-gray-600 dark:text-gray-300">Active</span>
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
