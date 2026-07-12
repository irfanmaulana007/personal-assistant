import { NavLink, Outlet } from 'react-router-dom';

const allSections = [
  { to: '/settings', label: 'Agent', end: true, admin: false },
  { to: '/settings/model', label: 'Model', end: false, admin: true },
  { to: '/settings/whatsapp', label: 'WhatsApp', end: false, admin: true },
  { to: '/settings/daily-skills', label: 'Daily Skills', end: false, admin: true },
  { to: '/settings/display', label: 'Display', end: false, admin: false },
  { to: '/settings/pricing', label: 'Pricing', end: false, admin: true },
];

export function Settings({ isAdmin }: { isAdmin: boolean }) {
  const sections = allSections.filter((s) => isAdmin || !s.admin);

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
          <ul className="flex gap-1 md:flex-col">
            {sections.map((s) => (
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
        </nav>

        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </div>
    </div>
  );
}
