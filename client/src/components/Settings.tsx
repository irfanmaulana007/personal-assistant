import { NavLink, Outlet } from 'react-router-dom';
import { useProjects } from '../contexts/project';

type Scope = 'system' | 'project';

interface Section {
  // Path relative to the settings base (e.g. 'project/members'; '' = the base).
  path: string;
  label: string;
  end?: boolean;
  admin?: boolean;
}

interface SectionGroup {
  label: string;
  scope: Scope;
  // When set, the whole group is only shown to managers of the active project
  // (project admin or global superadmin).
  projectAdmin?: boolean;
  sections: Section[];
}

// Settings live in two shells:
//   * the project shell (/:slug/settings) shows Project management + the user's
//     own preferences;
//   * the global shell (/settings) shows platform/System settings only.
const groups: SectionGroup[] = [
  {
    label: 'Project',
    scope: 'project',
    projectAdmin: true,
    sections: [
      { path: 'project', label: 'Overview', end: true },
      { path: 'project/members', label: 'Members' },
      { path: 'project/skills', label: 'Skills' },
      { path: 'project/features', label: 'Features' },
      // The LLM provider + skill API keys are per-project, so they live under
      // Project management (visible to project admins), not the System group.
      { path: 'model', label: 'Model' },
      { path: 'project/audit', label: 'Audit' },
    ],
  },
  {
    label: 'User',
    scope: 'project',
    sections: [
      { path: '', label: 'Agent', end: true },
      { path: 'display', label: 'Display' },
    ],
  },
  {
    label: 'System',
    scope: 'system',
    sections: [{ path: 'pricing', label: 'Pricing', admin: true }],
  },
];

export function Settings({ scope, isAdmin }: { scope: Scope; isAdmin: boolean }) {
  const { canManageActive, activeProject } = useProjects();

  // Section links are absolute. The project shell is prefixed by the active
  // project's slug; the system shell lives at the top level.
  const base = scope === 'project' ? `/${activeProject?.slug ?? ''}/settings` : '/settings';
  const to = (path: string) => (path ? `${base}/${path}` : base);

  const visibleGroups = groups
    .filter((g) => g.scope === scope)
    .filter((g) => !g.projectAdmin || canManageActive)
    .map((g) => ({ ...g, sections: g.sections.filter((s) => isAdmin || !s.admin) }))
    .filter((g) => g.sections.length > 0);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Settings
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        {scope === 'project'
          ? 'Manage your project and how your assistant works and looks.'
          : 'Platform-wide settings that apply across every project.'}
      </p>

      <div className="mt-6 flex flex-col gap-6 md:flex-row">
        <nav className="w-full shrink-0 md:w-48">
          <div className="flex flex-col gap-5">
            {visibleGroups.map((g) => (
              <div key={g.label}>
                <div className="mb-1 px-3 text-[11px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                  {g.label}
                </div>
                <ul className="flex flex-wrap gap-1 md:flex-col md:flex-nowrap">
                  {g.sections.map((s) => (
                    <li key={s.path || 'index'}>
                      <NavLink
                        to={to(s.path)}
                        end={s.end}
                        className={({ isActive }) =>
                          `block rounded-lg px-3 py-2 text-sm transition ${
                            isActive
                              ? 'bg-white font-medium text-indigo-700 dark:bg-gray-800 dark:text-indigo-300'
                              : 'font-normal text-gray-500 hover:bg-white/60 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-white/5 dark:hover:text-gray-100'
                          }`
                        }
                      >
                        {s.label}
                      </NavLink>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </nav>

        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </div>
    </div>
  );
}
