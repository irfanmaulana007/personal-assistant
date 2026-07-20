import { useEffect, useMemo, useState } from 'react';
import {
  listAdminSkills,
  setSkillCore,
  setAdminSkillPrompt,
  resetAdminSkillPrompt,
  revertAdminSkillTuned,
} from '../api/client';
import type { AdminSkill, SkillClassification } from '../types';
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

const TABS: { key: SkillClassification; label: string; blurb: string }[] = [
  {
    key: 'core',
    label: 'Core',
    blurb: 'Always available to every project. Toggleable per project by its admins.',
  },
  {
    key: 'global',
    label: 'Global',
    blurb: 'Shared skills available to every project.',
  },
  {
    key: 'project',
    label: 'Project-specific',
    blurb: 'Skills scoped to a single project — a project fork, or a skill only one project uses.',
  },
];

// Editor for a global skill's prompt (platform-wide). Remounted per skill via a
// `key` on the caller so the draft always starts from that skill.
function AdminSkillPromptModal({
  skill,
  onClose,
  onSaved,
}: {
  skill: AdminSkill;
  onClose: () => void;
  onSaved: (skills: AdminSkill[]) => void;
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
      onSaved(await setAdminSkillPrompt(skill.id, draft));
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
      const updated = await resetAdminSkillPrompt(skill.id);
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
      description="This is the instruction the assistant follows when this skill is active. It applies to every project."
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

// The projects a skill maps to, rendered as a compact wrapped list of chips.
function ProjectMapping({ skill }: { skill: AdminSkill }) {
  if (skill.projects.length === 0) {
    return (
      <span className="text-xs italic text-gray-400 dark:text-gray-500">
        Not enabled in any project
      </span>
    );
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-gray-400 dark:text-gray-500">In:</span>
      {skill.projects.map((p) => (
        <span
          key={p.id}
          className="inline-flex items-center rounded-md bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-gray-700/60 dark:text-gray-300"
        >
          {p.name}
        </span>
      ))}
    </div>
  );
}

export function AdminSkills() {
  const [skills, setSkills] = useState<AdminSkill[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [tab, setTab] = useState<SkillClassification>('core');
  const [coreBusyId, setCoreBusyId] = useState<number | null>(null);
  const [revertingId, setRevertingId] = useState<number | null>(null);
  const [editingId, setEditingId] = useState<number | null>(null);

  useEffect(() => {
    let active = true;
    listAdminSkills()
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

  const counts = useMemo(() => {
    const c: Record<SkillClassification, number> = { core: 0, global: 0, project: 0 };
    for (const s of skills) c[s.classification] += 1;
    return c;
  }, [skills]);

  const visible = useMemo(() => skills.filter((s) => s.classification === tab), [skills, tab]);

  const toggleCore = async (sk: AdminSkill) => {
    setCoreBusyId(sk.id);
    setError('');
    try {
      setSkills(await setSkillCore(sk.id, !sk.is_core));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update skill');
    } finally {
      setCoreBusyId(null);
    }
  };

  const revert = async (sk: AdminSkill) => {
    setRevertingId(sk.id);
    setError('');
    try {
      setSkills(await revertAdminSkillTuned(sk.id));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to revert skill prompt');
    } finally {
      setRevertingId(null);
    }
  };

  const editing = editingId != null ? skills.find((s) => s.id === editingId) : undefined;
  const activeTab = TABS.find((t) => t.key === tab)!;

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Skills
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        The platform-wide skill catalog across every project. A skill is core, global, or scoped to
        a single project depending on how projects use it. Project admins enable and customize the
        skills available to their project from its settings.
      </p>

      {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

      <div className="mt-5">
        <nav className="flex flex-wrap gap-1 border-b border-gray-200 dark:border-gray-700">
          {TABS.map((t) => (
            <button
              key={t.key}
              type="button"
              onClick={() => setTab(t.key)}
              className={`-mb-px flex items-center gap-2 border-b-2 px-3 py-2 text-sm font-medium transition ${
                tab === t.key
                  ? 'border-indigo-600 text-indigo-700 dark:border-indigo-400 dark:text-indigo-400'
                  : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
              }`}
            >
              {t.label}
              <span
                className={`inline-flex min-w-5 items-center justify-center rounded-full px-1.5 text-[11px] font-semibold ${
                  tab === t.key
                    ? 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300'
                    : 'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
                }`}
              >
                {counts[t.key]}
              </span>
            </button>
          ))}
        </nav>
      </div>

      <p className="mt-3 text-sm text-gray-500 dark:text-gray-400">{activeTab.blurb}</p>

      {loading ? (
        <div className="mt-4 space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonListRow key={i} />
          ))}
          <Skeleton className="h-2.5 w-24" />
        </div>
      ) : visible.length === 0 ? (
        <p className="mt-6 text-sm text-gray-500 dark:text-gray-400">
          No {activeTab.label.toLowerCase()} skills.
        </p>
      ) : (
        <div className="mt-4 divide-y divide-gray-100 overflow-hidden rounded-2xl border border-gray-200 bg-white dark:divide-gray-800 dark:border-gray-700 dark:bg-gray-800">
          {visible.map((sk) => {
            const isFork = sk.scope === 'project';
            return (
              <div key={sk.id} className="flex items-start gap-4 p-4">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                      {sk.name}
                    </span>
                    <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-500 dark:bg-gray-700 dark:text-gray-400">
                      {sk.category}
                    </span>
                    {isFork && (
                      <span
                        title="A project-owned fork with a prompt customized for that project."
                        className="inline-flex items-center rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 ring-1 ring-inset ring-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-500/30"
                      >
                        Fork
                      </span>
                    )}
                    {sk.auto_tuned && (
                      <span
                        title="The end-of-day self-tuner has refined this skill's instructions."
                        className="inline-flex items-center rounded-full bg-indigo-50 px-2 py-0.5 text-[11px] font-medium text-indigo-700 ring-1 ring-inset ring-indigo-200 dark:bg-indigo-500/10 dark:text-indigo-300 dark:ring-indigo-500/30"
                      >
                        Auto-tuned
                      </span>
                    )}
                  </div>
                  <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
                    {sk.description}
                  </p>
                  <div className="mt-2">
                    <ProjectMapping skill={sk} />
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-3">
                    {!isFork && (
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
                    )}
                    {isFork && (
                      <span className="text-xs italic text-gray-400 dark:text-gray-500">
                        Customized in {sk.projects[0]?.name ?? 'its project'} — edit from that
                        project’s settings.
                      </span>
                    )}
                    {sk.auto_tuned && (
                      <button
                        type="button"
                        onClick={() => revert(sk)}
                        disabled={revertingId === sk.id}
                        className="text-xs font-medium text-indigo-700 hover:text-indigo-800 disabled:opacity-50 dark:text-indigo-400 dark:hover:text-indigo-300"
                      >
                        {revertingId === sk.id ? 'Reverting…' : 'Revert to default prompt'}
                      </button>
                    )}
                    {sk.prompt_updated_at && !isFork && (
                      <span className="text-xs text-gray-400 dark:text-gray-500">
                        Edited {formatEdited(sk.prompt_updated_at)}
                        {sk.prompt_updated_by ? ` by ${sk.prompt_updated_by}` : ''}
                      </span>
                    )}
                  </div>
                </div>
                {/* Only global skills can be flagged core; a fork is always project-specific. */}
                {!isFork && (
                  <div className="flex shrink-0 flex-col items-center gap-1">
                    <span className="text-[11px] font-medium text-gray-500 dark:text-gray-400">
                      Core
                    </span>
                    <Toggle
                      on={sk.is_core}
                      busy={coreBusyId === sk.id}
                      onClick={() => toggleCore(sk)}
                    />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {editing && editing.scope === 'global' && (
        <AdminSkillPromptModal
          key={editing.id}
          skill={editing}
          onClose={() => setEditingId(null)}
          onSaved={setSkills}
        />
      )}
    </div>
  );
}
