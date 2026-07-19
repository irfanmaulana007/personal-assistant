import { NavLink, Outlet } from 'react-router-dom';

type SettingsGroup = 'user' | 'project' | 'system';

const allSections: {
  to: string;
  label: string;
  end: boolean;
  admin: boolean;
  group: SettingsGroup;
}[] = [
  { to: '/settings', label: 'Agent', end: true, admin: false, group: 'user' },
  { to: '/settings/display', label: 'Display', end: false, admin: false, group: 'user' },
  {
    to: '/settings/whatsapp-mappings',
    label: 'WhatsApp Projects',
    end: false,
    admin: true,
    group: 'project',
  },
  { to: '/settings/model', label: 'Model', end: false, admin: true, group: 'system' },
  { to: '/settings/api-keys', label: 'API Keys', end: false, admin: true, group: 'system' },
  { to: '/settings/whatsapp', label: 'WhatsApp', end: false, admin: true, group: 'system' },
  { to: '/settings/daily-skills', label: 'Daily Skills', end: false, admin: true, group: 'system' },
  { to: '/settings/pricing', label: 'Pricing', end: false, admin: true, group: 'system' },
];

const groupOrder: { key: SettingsGroup; label: string }[] = [
  { key: 'user', label: 'User' },
  { key: 'project', label: 'Project' },
  { key: 'system', label: 'System' },
];

export function Settings({ isAdmin }: { isAdmin: boolean }) {
  const sections = allSections.filter((s) => isAdmin || !s.admin);
  const groups = groupOrder
    .map((g) => ({ ...g, items: sections.filter((s) => s.group === g.key) }))
    .filter((g) => g.items.length > 0);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-100 p-6 dark:bg-gray-900">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
        Settings
      </h1>
      <p className="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
        Manage how your assistant works and looks.
      </p>

      <div className="mt-6 flex flex-col gap-6 md:flex-row">
        <nav className="w-full shrink-0 md:w-48">
          <div className="flex flex-col gap-5">
            {groups.map((group) => (
              <div key={group.key}>
                <p className="px-3 pb-1.5 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
                  {group.label}
                </p>
                <ul className="flex gap-1 md:flex-col">
                  {group.items.map((s) => (
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
