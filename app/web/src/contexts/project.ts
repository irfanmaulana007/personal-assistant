import { createContext, useContext } from 'react';
import type { Project, ProjectFeature } from '../types';

export interface ProjectValue {
  projects: Project[];
  activeProject: Project | null;
  loading: boolean;
  // Switch the active project. Persists the choice, navigates to the project's
  // /:slug URL and reloads so every project-scoped view refetches under the new
  // X-Project-Id.
  setActiveProject: (id: number) => void;
  // Build a path inside the active project's shell, e.g. projectPath('chat') →
  // '/acme/chat'. Returns '/' when there is no active project.
  projectPath: (sub?: string) => string;
  reload: () => void;
  // Whether the caller can manage the active project (project admin or superadmin).
  canManageActive: boolean;
  // The active project's features + whether its nav item should show. A feature's
  // nav is visible only when the feature is enabled AND (it owns no skills, or at
  // least one of its skills is enabled for the project) — so disabling a project
  // skill/feature hides its navigation entry.
  features: ProjectFeature[];
  navFeatureVisible: (featureKey: string) => boolean;
}

export const ProjectCtx = createContext<ProjectValue | null>(null);

export function useProjects(): ProjectValue {
  const v = useContext(ProjectCtx);
  if (!v) throw new Error('useProjects must be used within a ProjectProvider');
  return v;
}
