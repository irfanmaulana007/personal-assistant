import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useProjects } from '../contexts/project';
import { CreateProjectModal } from './CreateProjectModal';
import { SkeletonCard } from './ui/Skeleton';

// Projects is the global-shell landing that lists the projects the caller can
// reach and lets them enter one. Superadmins see every project and can create a
// new one; members see only their own projects (this doubles as their picker).
export function Projects({ isAdmin }: { isAdmin: boolean }) {
  const { projects, loading } = useProjects();
  const navigate = useNavigate();
  const [creating, setCreating] = useState(false);

  const enter = (slug: string) => navigate(`/${slug}`);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
            Projects
          </h1>
          <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
            {isAdmin
              ? 'Every project on the platform. Open one to manage its chat, analytics, and settings.'
              : 'Open a project to work inside it.'}
          </p>
        </div>
        {isAdmin && (
          <button
            onClick={() => setCreating(true)}
            className="inline-flex items-center gap-2 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-500 dark:bg-indigo-500 dark:hover:bg-indigo-400"
          >
            <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 4v16m8-8H4"
              />
            </svg>
            New project
          </button>
        )}
      </div>

      <div className="mt-6">
        {loading ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <SkeletonCard key={i}>
                <div className="h-16" />
              </SkeletonCard>
            ))}
          </div>
        ) : projects.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-gray-300 bg-white p-10 text-center dark:border-gray-700 dark:bg-gray-800">
            <p className="text-sm font-medium text-gray-900 dark:text-gray-100">No projects yet</p>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {isAdmin
                ? 'Create your first project to get started.'
                : 'You have not been added to any projects yet.'}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {projects.map((p) => (
              <button
                key={p.id}
                onClick={() => enter(p.slug)}
                title={`Open ${p.name}`}
                className="group flex items-center gap-3 rounded-2xl border border-gray-200 bg-white p-4 text-left transition hover:border-indigo-300 hover:shadow-sm dark:border-gray-700 dark:bg-gray-800 dark:hover:border-indigo-500/50"
              >
                <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-indigo-500/15 text-base font-semibold text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300">
                  {(p.name || '?').charAt(0).toUpperCase()}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-semibold text-gray-900 dark:text-gray-50">
                    {p.name}
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span className="capitalize">{p.role}</span>
                    <span aria-hidden>·</span>
                    <span>
                      {p.member_count} member{p.member_count === 1 ? '' : 's'}
                    </span>
                  </div>
                </div>
                <span
                  aria-hidden
                  className="shrink-0 text-gray-300 transition group-hover:translate-x-0.5 group-hover:text-indigo-600 dark:text-gray-600 dark:group-hover:text-indigo-400"
                >
                  →
                </span>
              </button>
            ))}
          </div>
        )}
      </div>

      <CreateProjectModal open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}
