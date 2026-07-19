import { useState } from 'react';
import * as Popover from '@radix-ui/react-popover';
import { useNavigate } from 'react-router-dom';
import { useProjects } from '../contexts/project';
import { CreateProjectModal } from './CreateProjectModal';

// Pinned at the top of the sidebar (Dokploy-style): shows the active project and
// lets the user switch between projects, jump into its settings, or (superadmin)
// create a new one. Switching reloads so every project-scoped view refetches
// under the new project.
export function ProjectSwitcher({ isSuperadmin }: { isSuperadmin: boolean }) {
  const navigate = useNavigate();
  const { projects, activeProject, setActiveProject, loading, canManageActive } = useProjects();
  const [creating, setCreating] = useState(false);

  const label = loading ? 'Loading…' : activeProject?.name || 'No project';

  return (
    <>
      <Popover.Root>
        <Popover.Trigger asChild>
          <button className="flex w-full items-center gap-2.5 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-left transition hover:bg-white/10">
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-indigo-500/20 text-xs font-semibold text-indigo-300">
              {(activeProject?.name || '?').charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[10px] font-medium uppercase tracking-wide text-slate-500">
                Project
              </div>
              <div className="truncate text-sm font-medium text-white">{label}</div>
            </div>
            <svg
              className="h-4 w-4 shrink-0 text-slate-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M8 9l4-4 4 4m0 6l-4 4-4-4"
              />
            </svg>
          </button>
        </Popover.Trigger>

        <Popover.Portal>
          <Popover.Content
            side="bottom"
            align="start"
            sideOffset={8}
            className="z-50 max-h-[60vh] w-[228px] overflow-y-auto rounded-xl border border-gray-200 bg-white p-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
          >
            <div className="px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
              Switch project
            </div>
            {projects.length === 0 && (
              <div className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">
                No projects yet
              </div>
            )}
            <div className="p-1">
              {projects.map((p) => {
                const active = activeProject?.id === p.id;
                return (
                  <Popover.Close asChild key={p.id}>
                    <button
                      onClick={() => !active && setActiveProject(p.id)}
                      className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
                        active
                          ? 'bg-indigo-50 font-medium text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
                          : 'text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700/60'
                      }`}
                    >
                      <span className="min-w-0 flex-1 truncate">{p.name}</span>
                      <span className="shrink-0 text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {p.role}
                      </span>
                      {active && (
                        <svg
                          className="h-4 w-4 shrink-0 text-indigo-600 dark:text-indigo-400"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M5 13l4 4L19 7"
                          />
                        </svg>
                      )}
                    </button>
                  </Popover.Close>
                );
              })}
            </div>
            <div className="border-t border-gray-100 p-1 dark:border-gray-700">
              {canManageActive && activeProject && (
                <Popover.Close asChild>
                  <button
                    onClick={() => navigate('/settings/project')}
                    className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm font-medium text-gray-700 transition hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700/60"
                  >
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
                        d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                      />
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                      />
                    </svg>
                    Project settings
                  </button>
                </Popover.Close>
              )}
              {isSuperadmin && (
                <Popover.Close asChild>
                  <button
                    onClick={() => setCreating(true)}
                    className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm font-medium text-indigo-700 transition hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-500/15"
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
                </Popover.Close>
              )}
            </div>
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>

      <CreateProjectModal open={creating} onClose={() => setCreating(false)} />
    </>
  );
}
