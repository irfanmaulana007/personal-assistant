import { useState, useEffect } from 'react';
import { getRoutines, updateRoutine, runRoutine } from '../../api/client';
import { SkeletonFormCard } from '../ui/Skeleton';
import { Toggle } from '../ui/Toggle';
import type { Routine } from '../../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

type Note = { ok: boolean; text: string };

function RoutineCard({ routine, onChange }: { routine: Routine; onChange: (r: Routine) => void }) {
  // Local draft for the free-form fields (time + prompt); the toggle saves
  // immediately, while time/prompt are committed with the Save button.
  const [time, setTime] = useState(routine.time);
  const [prompt, setPrompt] = useState(routine.prompt);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [note, setNote] = useState<Note | null>(null);

  const dirty = time !== routine.time || prompt !== routine.prompt;
  const isDefaultPrompt = prompt.trim() === routine.default_prompt.trim();

  const toggle = async () => {
    setNote(null);
    try {
      onChange(await updateRoutine(routine.key, { enabled: !routine.enabled }));
    } catch (e) {
      setNote({ ok: false, text: e instanceof Error ? e.message : 'Failed to update' });
    }
  };

  const save = async () => {
    setSaving(true);
    setNote(null);
    try {
      onChange(await updateRoutine(routine.key, { time, prompt }));
      setNote({ ok: true, text: 'Saved.' });
    } catch (e) {
      setNote({ ok: false, text: e instanceof Error ? e.message : 'Failed to save' });
    } finally {
      setSaving(false);
    }
  };

  const run = async () => {
    setRunning(true);
    setNote(null);
    try {
      const res = await runRoutine(routine.key);
      setNote({
        ok: true,
        text: res.sent ? 'Sent to WhatsApp.' : 'Ran — nothing to report, so no message was sent.',
      });
    } catch (e) {
      setNote({ ok: false, text: e instanceof Error ? e.message : 'Failed to run' });
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h3 className="text-base font-semibold text-gray-900 dark:text-gray-50">
            {routine.name}
          </h3>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{routine.description}</p>
        </div>
        <Toggle on={routine.enabled} onClick={toggle} />
      </div>

      <div className="mt-5 space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-200">
            Run time
          </label>
          <input
            type="time"
            value={time}
            onChange={(e) => setTime(e.target.value)}
            className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:ring-indigo-500/30"
          />
          <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
            Local time this skill runs each day.
          </p>
        </div>

        <div>
          <div className="mb-1 flex items-center justify-between">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-200">
              Prompt
            </label>
            {!isDefaultPrompt && (
              <button
                type="button"
                onClick={() => setPrompt(routine.default_prompt)}
                className="text-xs font-medium text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300"
              >
                Reset to default
              </button>
            )}
          </div>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={8}
            className={`${inputClass} resize-y font-mono text-xs leading-relaxed`}
          />
          <p className="mt-1 text-xs text-gray-400 dark:text-gray-500">
            The instructions the assistant follows when this skill runs. It has full tool access
            (reminders, calendar, and more) and its reply is what gets sent to you.
          </p>
        </div>
      </div>

      {note && (
        <p
          className={`mt-4 text-sm ${note.ok ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}
        >
          {note.text}
        </p>
      )}

      <div className="mt-5 flex items-center gap-3">
        <button
          type="button"
          onClick={save}
          disabled={saving || !dirty}
          className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button
          type="button"
          onClick={run}
          disabled={running}
          className="rounded-xl border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
        >
          {running ? 'Running…' : 'Run now'}
        </button>
        {routine.last_run && (
          <span className="text-xs text-gray-400 dark:text-gray-500">
            Last ran {routine.last_run}
          </span>
        )}
      </div>
    </div>
  );
}

export function RoutinesSettings() {
  const [routines, setRoutines] = useState<Routine[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    getRoutines()
      .then((rs) => active && setRoutines(rs))
      .catch((e) => active && setError(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const replace = (r: Routine) => setRoutines((rs) => rs.map((x) => (x.key === r.key ? r : x)));

  if (loading) return <SkeletonFormCard fields={4} />;

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-semibold text-gray-900 dark:text-gray-50">Daily skills</h2>
        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
          Scheduled prompts that run once a day through your assistant and send you the result over
          WhatsApp. Turn one on, set its time, and tailor its prompt.
        </p>
      </div>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {routines.map((r) => (
        <RoutineCard key={r.key} routine={r} onChange={replace} />
      ))}
    </div>
  );
}
