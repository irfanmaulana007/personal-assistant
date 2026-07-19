import { useState, useEffect, type ReactNode } from 'react';
import { useLocation } from 'react-router-dom';
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
  const location = useLocation();
  const [projects, setProjects] = useState<Project[]>([]);
  // Seeded once from the persisted X-Project-Id; the URL slug takes precedence
  // over it when we're inside a project shell. Kept as a stable fallback so a
  // global page (no slug) still resolves the last active project.
  const [activeId] = useState<number | null>(getActiveProjectId());
  const [loading, setLoading] = useState(true);
  const [features, setFeatures] = useState<ProjectFeature[]>([]);
  const [enabledSkillKeys, setEnabledSkillKeys] = useState<Set<string>>(new Set());
  const [tick, setTick] = useState(0);

  // First path segment, e.g. "acme" from /acme/chat. When it matches a project's
  // slug we're inside that project's shell; otherwise (or on a global page like
  // /overview) it's empty for our purposes.
  const urlSlug = location.pathname.split('/')[1] ?? '';

  useEffect(() => {
    let active = true;
    listProjects()
      .then((ps) => {
        if (!active) return;
        setProjects(ps);
      })
      .catch(() => {})
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [tick]);

  // The active project is resolved from the URL slug when we're inside a project
  // shell, else from the persisted id, else the first project.
  const projectBySlug = projects.find((p) => p.slug === urlSlug) ?? null;
  const activeProject =
    projectBySlug ?? projects.find((p) => p.id === activeId) ?? projects[0] ?? null;
  const activeProjectId = activeProject?.id ?? null;

  // Keep the persisted X-Project-Id in sync with the resolved active project.
  // When the URL points at a different project than the stored header, adopt it
  // and hard-reload once so every scoped fetch runs under the right project.
  const bySlugId = projectBySlug?.id ?? null;
  useEffect(() => {
    if (loading || activeProjectId == null) return;
    if (getActiveProjectId() !== activeProjectId) {
      setActiveProjectId(activeProjectId);
      // Only the URL forcing a different project warrants a reload; a silent
      // catch-up (no active project stored yet) just persists the id.
      if (bySlugId != null && bySlugId === activeProjectId) {
        window.location.reload();
      }
    }
  }, [loading, activeProjectId, bySlugId]);

  // Load the active project's features + enabled skills so the nav can hide
  // feature items whose feature/skill is disabled for this project.
  useEffect(() => {
    let active = true;
    if (activeProjectId == null) return;
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
  }, [activeProjectId, tick]);

  // A feature's nav item is visible when the feature is enabled AND (it owns no
  // skills, or at least one of its skills is enabled for the project).
  const navFeatureVisible = (featureKey: string): boolean => {
    const f = features.find((x) => x.key === featureKey);
    if (!f) return true; // unknown key → don't hide (feature catalog not loaded yet)
    if (!f.enabled) return false;
    if (f.skill_keys.length === 0) return true;
    return f.skill_keys.some((k) => enabledSkillKeys.has(k));
  };

  const projectPath = (sub = ''): string => {
    const slug = activeProject?.slug;
    if (!slug) return '/';
    const clean = sub.replace(/^\/+/, '');
    return clean ? `/${slug}/${clean}` : `/${slug}`;
  };

  const setActiveProject = (id: number) => {
    const p = projects.find((x) => x.id === id);
    if (!p) return;
    setActiveProjectId(id);
    // Preserve the current sub-path when switching from inside a project shell
    // (e.g. /old/dashboard → /new/dashboard); otherwise land on the project root.
    const segs = location.pathname.split('/').filter(Boolean);
    const inProjectShell = projects.some((pr) => pr.slug === segs[0]);
    const rest = inProjectShell ? segs.slice(1).join('/') : '';
    // Full navigation (not client-side) so every project-scoped view refetches.
    window.location.href = rest ? `/${p.slug}/${rest}` : `/${p.slug}`;
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
        projectPath,
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
