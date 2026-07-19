import { createContext, useContext } from 'react';
import type { Project } from '../types';

export interface ProjectValue {
  projects: Project[];
  activeProject: Project | null;
  loading: boolean;
  // Switch the active project. Persists the choice and reloads so every
  // project-scoped view refetches under the new X-Project-Id.
  setActiveProject: (id: number) => void;
  reload: () => void;
  // Whether the caller can manage the active project (project admin or superadmin).
  canManageActive: boolean;
}

export const ProjectCtx = createContext<ProjectValue | null>(null);

export function useProjects(): ProjectValue {
  const v = useContext(ProjectCtx);
  if (!v) throw new Error('useProjects must be used within a ProjectProvider');
  return v;
}
