import { useState, useEffect } from 'react';
import { listSkills, setSkillEnabled, setSkillPrompt } from '../api/client';
import type { Skill } from '../types';
import { Toggle } from './ui/Toggle';
import { Skeleton, SkeletonListRow } from './ui/Skeleton';

export function Skills({ isAdmin = false }: { isAdmin?: boolean }) {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);

  // Prompt editor state (admins only): the skill being edited, its draft text,
  // and whether a save is in flight.
  const [editingId, setEditingId] = useState<number | null>(null);
  const [draft, setDraft] = useState('');
  const [savingPrompt, setSavingPrompt] = useState(false);

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

  const startEdit = (sk: Skill) => {
    setEditingId(sk.id);
    setDraft(sk.prompt ?? '');
    setError('');
  };

  const cancelEdit = () => {
    setEditingId(null);
    setDraft('');
  };

  const savePrompt = async (sk: Skill) => {
    setSavingPrompt(true);
    setError('');
    try {
      setSkills(await setSkillPrompt(sk.id, draft));
      setEditingId(null);
      setDraft('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save prompt');
    } finally {
      setSavingPrompt(false);
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

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Skills
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        Turn skills on to give the assistant new abilities. Changes apply to your account.
        {isAdmin && ' As an admin you can also edit each skill’s system prompt for everyone.'}
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
                  <div key={sk.id} className="p-4">
                    <div className="flex items-start gap-4">
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">
                          {sk.name}
                        </div>
                        <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
                          {sk.description}
                        </p>
                      </div>
                      <Toggle on={sk.enabled} busy={busyId === sk.id} onClick={() => toggle(sk)} />
                    </div>

                    {isAdmin &&
                      (editingId === sk.id ? (
                        <div className="mt-3">
                          <label className="mb-1 block text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                            System prompt
                          </label>
                          <textarea
                            value={draft}
                            onChange={(e) => setDraft(e.target.value)}
                            rows={6}
                            placeholder="This skill has no system-prompt fragment. Leave empty if it is handled deterministically."
                            className="w-full resize-y rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder:text-gray-400 dark:placeholder:text-gray-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
                          />
                          <div className="mt-2 flex items-center gap-2">
                            <button
                              onClick={() => savePrompt(sk)}
                              disabled={savingPrompt}
                              className="rounded-lg bg-indigo-600 dark:bg-indigo-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 dark:hover:bg-indigo-400 disabled:opacity-60"
                            >
                              {savingPrompt ? 'Saving…' : 'Save'}
                            </button>
                            <button
                              onClick={cancelEdit}
                              disabled={savingPrompt}
                              className="rounded-lg border border-gray-300 dark:border-gray-600 px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-60"
                            >
                              Cancel
                            </button>
                          </div>
                        </div>
                      ) : (
                        <div className="mt-3">
                          {sk.prompt ? (
                            <p className="whitespace-pre-wrap rounded-lg bg-gray-50 dark:bg-gray-900/60 p-3 text-xs leading-relaxed text-gray-600 dark:text-gray-300">
                              {sk.prompt}
                            </p>
                          ) : (
                            <p className="text-xs italic text-gray-400 dark:text-gray-500">
                              No system-prompt fragment.
                            </p>
                          )}
                          <button
                            onClick={() => startEdit(sk)}
                            className="mt-2 text-xs font-medium text-indigo-700 dark:text-indigo-400 hover:underline"
                          >
                            Edit prompt
                          </button>
                        </div>
                      ))}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
