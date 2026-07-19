import { useState, useEffect, type ReactNode } from 'react';
import { listProjects, getActiveProjectId, setActiveProjectId } from '../api/client';
import type { Project } from '../types';
import { ProjectCtx } from './project';

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [activeId, setActiveId] = useState<number | null>(getActiveProjectId());
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let active = true;
    listProjects()
      .then((ps) => {
        if (!active) return;
        setProjects(ps);
        // Ensure a valid active project is selected (stored one, else the first).
        const stored = getActiveProjectId();
        const chosen = ps.find((p) => p.id === stored)?.id ?? ps[0]?.id ?? null;
        if (chosen !== stored) setActiveProjectId(chosen);
        setActiveId(chosen);
      })
      .catch(() => {})
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [tick]);

  const activeProject = projects.find((p) => p.id === activeId) ?? null;

  const setActiveProject = (id: number) => {
    setActiveProjectId(id);
    setActiveId(id);
    // Reload so every project-scoped page refetches under the new project.
    window.location.reload();
  };

  const canManageActive = activeProject
    ? activeProject.role === 'admin' || activeProject.role === 'superadmin'
    : false;

  return (
    <ProjectCtx.Provider
      value={{
        projects,
        activeProject,
        loading,
        setActiveProject,
        reload: () => setTick((t) => t + 1),
        canManageActive,
      }}
    >
      {children}
    </ProjectCtx.Provider>
  );
}
