import { useState, useEffect } from 'react';
import { listSkills, setSkillEnabled, setSkillPrompt, resetSkillPrompt } from '../api/client';
import type { Skill } from '../types';
import { Toggle } from './ui/Toggle';
import { Modal } from './ui/Modal';
import { Skeleton, SkeletonListRow } from './ui/Skeleton';

const textareaClass =
  'w-full resize-y rounded-xl border border-gray-200 px-3 py-2.5 font-mono text-xs leading-relaxed text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:focus:border-indigo-400 dark:focus:ring-indigo-500/30';

function formatEdited(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

// Admin-only editor for a skill's prompt, shown in a modal. Remounted per skill
// (via a `key` on the caller) so the draft always starts from that skill.
function SkillPromptModal({
  skill,
  onClose,
  onSaved,
}: {
  skill: Skill;
  onClose: () => void;
  onSaved: (skills: Skill[]) => void;
}) {
  const [draft, setDraft] = useState(skill.prompt ?? '');
  const [saving, setSaving] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null);

  const customized = !!skill.prompt_updated_at;
  const dirty = draft !== (skill.prompt ?? '');
  const empty = draft.trim() === '';
  const busy = saving || resetting;

  const save = async () => {
    setSaving(true);
    setNote(null);
    try {
      onSaved(await setSkillPrompt(skill.id, draft));
      setNote({ ok: true, text: 'Prompt saved.' });
    } catch (e) {
      setNote({ ok: false, text: e instanceof Error ? e.message : 'Failed to save prompt' });
    } finally {
      setSaving(false);
    }
  };

  const restoreDefault = async () => {
    setResetting(true);
    setNote(null);
    try {
      const updated = await resetSkillPrompt(skill.id);
      onSaved(updated);
      const fresh = updated.find((s) => s.id === skill.id);
      setDraft(fresh?.prompt ?? skill.default_prompt ?? '');
      setNote({ ok: true, text: 'Restored the default prompt.' });
    } catch (e) {
      setNote({ ok: false, text: e instanceof Error ? e.message : 'Failed to restore default' });
    } finally {
      setResetting(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={`Edit prompt · ${skill.name}`}
      description="This is the instruction the assistant follows when this skill is active. It applies to everyone."
    >
      <div className="space-y-4">
        <div className="rounded-xl border border-gray-200 bg-gray-50 px-3 py-2.5 text-xs text-gray-500 dark:border-gray-700 dark:bg-gray-900/50 dark:text-gray-400">
          {customized ? (
            <>
              Last edited {skill.prompt_updated_at && formatEdited(skill.prompt_updated_at)}
              {skill.prompt_updated_by ? (
                <>
                  {' '}
                  by{' '}
                  <span className="font-medium text-gray-700 dark:text-gray-200">
                    {skill.prompt_updated_by}
                  </span>
                </>
              ) : null}
              .
            </>
          ) : (
            <>Using the built-in default prompt — it hasn’t been customized yet.</>
          )}
        </div>

        <div>
          <div className="mb-1 flex items-center justify-between">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-200">
              Prompt
            </label>
            {customized && (
              <button
                type="button"
                onClick={restoreDefault}
                disabled={busy}
                className="text-xs font-medium text-indigo-600 transition hover:text-indigo-700 disabled:opacity-50 dark:text-indigo-400 dark:hover:text-indigo-300"
              >
                {resetting ? 'Restoring…' : 'Restore default'}
              </button>
            )}
          </div>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            rows={12}
            className={textareaClass}
          />
        </div>

        {note && (
          <p
            className={`text-sm ${note.ok ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}
          >
            {note.text}
          </p>
        )}

        <div className="flex items-center justify-end gap-3 pt-1">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50 dark:border-gray-600 dark:text-gray-200 dark:hover:bg-gray-700"
          >
            Close
          </button>
          <button
            type="button"
            onClick={save}
            disabled={busy || !dirty || empty}
            className="rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-600"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export function Skills({ isAdmin }: { isAdmin: boolean }) {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);
  const [editingId, setEditingId] = useState<number | null>(null);

  useEffect(() => {
    let active = true;
    listSkills()
      .then((s) => {
        if (active) setSkills(s);
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load skills');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const toggle = async (sk: Skill) => {
    setBusyId(sk.id);
    setError('');
    try {
      setSkills(await setSkillEnabled(sk.id, !sk.enabled));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update skill');
    } finally {
      setBusyId(null);
    }
  };

  // Group by category, preserving order.
  const groups: { category: string; skills: Skill[] }[] = [];
  for (const sk of skills) {
    const cat = sk.category || 'Other';
    let g = groups.find((x) => x.category === cat);
    if (!g) {
      g = { category: cat, skills: [] };
      groups.push(g);
    }
    g.skills.push(sk);
  }

  const editing = editingId != null ? skills.find((s) => s.id === editingId) : undefined;

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Skills
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        Turn skills on to give the assistant new abilities. Changes apply to your account.
        {isAdmin && ' As an admin you can also edit each skill’s prompt for everyone.'}
      </p>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <div className="mt-6 space-y-6">
          {[3, 2].map((count, g) => (
            <div key={g}>
              <Skeleton className="mb-2 h-2.5 w-24" />
              <div className="space-y-2">
                {Array.from({ length: count }).map((_, i) => (
                  <SkeletonListRow key={i} />
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-6 space-y-6">
          {groups.map((g) => (
            <div key={g.category}>
              <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                {g.category}
              </h2>
              <div className="divide-y divide-gray-100 dark:divide-gray-800 overflow-hidden rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                {g.skills.map((sk) => (
                  <div key={sk.id} className="flex items-start gap-4 p-4">
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                        {sk.name}
                      </div>
                      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
                        {sk.description}
                      </p>
                      {isAdmin && (
                        <div className="mt-2 flex items-center gap-2">
                          <button
                            type="button"
                            onClick={() => setEditingId(sk.id)}
                            className="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 px-2.5 py-1 text-xs font-medium text-gray-600 transition hover:bg-gray-50 hover:text-gray-800 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-gray-100"
                          >
                            <svg
                              className="h-3.5 w-3.5"
                              viewBox="0 0 24 24"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth={2}
                            >
                              <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                d="M11 4H4v16h16v-7M18.5 2.5a2.12 2.12 0 013 3L12 15l-4 1 1-4 9.5-9.5z"
                              />
                            </svg>
                            Edit prompt
                          </button>
                          {sk.prompt_updated_at && (
                            <span className="text-xs text-gray-400 dark:text-gray-500">
                              Edited {formatEdited(sk.prompt_updated_at)}
                              {sk.prompt_updated_by ? ` by ${sk.prompt_updated_by}` : ''}
                            </span>
                          )}
                        </div>
                      )}
                    </div>
                    <Toggle on={sk.enabled} busy={busyId === sk.id} onClick={() => toggle(sk)} />
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {isAdmin && editing && (
        <SkillPromptModal
          key={editing.id}
          skill={editing}
          onClose={() => setEditingId(null)}
          onSaved={setSkills}
        />
      )}
    </div>
  );
}
