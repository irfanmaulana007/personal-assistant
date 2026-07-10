import { useState, useEffect } from 'react';
import { listSkills, setSkillEnabled } from '../api/client';
import type { Skill } from '../types';
import { Toggle } from './ui/Toggle';
import { Skeleton, SkeletonListRow } from './ui/Skeleton';

export function Skills() {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<number | null>(null);

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

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 dark:bg-gray-900 p-6">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Skills
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        Turn skills on to give the assistant new abilities. Changes apply to your account.
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
                    </div>
                    <Toggle on={sk.enabled} busy={busyId === sk.id} onClick={() => toggle(sk)} />
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
