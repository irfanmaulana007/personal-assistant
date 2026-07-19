import { NavLink, Outlet } from 'react-router-dom';
import { useProjects } from '../contexts/project';

interface Section {
  to: string;
  label: string;
  end?: boolean;
  admin?: boolean;
}

interface SectionGroup {
  label: string;
  // When set, the whole group is only shown to managers of the active project
  // (project admin or global superadmin).
  projectAdmin?: boolean;
  sections: Section[];
}

// Settings are grouped: the active project's management lives up top (visible
// only to that project's admins), the assistant/platform settings below.
const groups: SectionGroup[] = [
  {
    label: 'Project',
    projectAdmin: true,
    sections: [
      { to: '/settings/project', label: 'Overview', end: true },
      { to: '/settings/project/members', label: 'Members' },
      { to: '/settings/project/skills', label: 'Skills' },
      { to: '/settings/project/features', label: 'Features' },
      { to: '/settings/project/audit', label: 'Audit' },
    ],
  },
  {
    label: 'Assistant',
    sections: [
      { to: '/settings', label: 'Agent', end: true },
      { to: '/settings/model', label: 'Model', admin: true },
      { to: '/settings/api-keys', label: 'API Keys', admin: true },
      { to: '/settings/whatsapp', label: 'WhatsApp', admin: true },
      { to: '/settings/whatsapp-mappings', label: 'WhatsApp Projects', admin: true },
      { to: '/settings/display', label: 'Display' },
      { to: '/settings/pricing', label: 'Pricing', admin: true },
    ],
  },
];

export function Settings({ isAdmin }: { isAdmin: boolean }) {
  const { canManageActive } = useProjects();

  const visibleGroups = groups
    .filter((g) => !g.projectAdmin || canManageActive)
    .map((g) => ({ ...g, sections: g.sections.filter((s) => isAdmin || !s.admin) }))
    .filter((g) => g.sections.length > 0);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Settings
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        Manage your project and how your assistant works and looks.
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
                    <li key={s.to}>
                      <NavLink
                        to={s.to}
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
