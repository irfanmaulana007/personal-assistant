import { useState, useEffect, type ReactNode } from 'react';
import {
  listProjects,
  listFeatures,
  listSkills,
  getActiveProjectId,
  setActiveProjectId,
} from '../api/client';
import type { Project, ProjectFeature } from '../types';
import { ProjectCtx } from './project';

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [activeId, setActiveId] = useState<number | null>(getActiveProjectId());
  const [loading, setLoading] = useState(true);
  const [features, setFeatures] = useState<ProjectFeature[]>([]);
  const [enabledSkillKeys, setEnabledSkillKeys] = useState<Set<string>>(new Set());
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

  // Load the active project's features + enabled skills so the nav can hide
  // feature items whose feature/skill is disabled for this project.
  useEffect(() => {
    let active = true;
    if (!activeId) return;
    Promise.all([listFeatures().catch(() => []), listSkills().catch(() => [])])
      .then(([f, s]) => {
        if (!active) return;
        setFeatures(f);
        setEnabledSkillKeys(new Set(s.filter((sk) => sk.enabled).map((sk) => sk.key)));
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [activeId, tick]);

  const activeProject = projects.find((p) => p.id === activeId) ?? null;

  // A feature's nav item is visible when the feature is enabled AND (it owns no
  // skills, or at least one of its skills is enabled for the project).
  const navFeatureVisible = (featureKey: string): boolean => {
    const f = features.find((x) => x.key === featureKey);
    if (!f) return true; // unknown key → don't hide (feature catalog not loaded yet)
    if (!f.enabled) return false;
    if (f.skill_keys.length === 0) return true;
    return f.skill_keys.some((k) => enabledSkillKeys.has(k));
  };

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
        features,
        navFeatureVisible,
      }}
    >
      {children}
    </ProjectCtx.Provider>
  );
}
