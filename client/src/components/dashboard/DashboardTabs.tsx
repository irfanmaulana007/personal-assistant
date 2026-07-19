import { NavLink } from 'react-router-dom';

// The dashboard's tab set, rendered as an in-body tab bar (shared by the
// per-project dashboard and the global all-projects dashboard). Tabs link
// relative to a `base` path and carry the current `search` so date/channel/
// project filters survive tab switches.
const TABS: { to: string; label: string; end?: boolean }[] = [
  { to: '', label: 'Overview', end: true },
  { to: 'usage', label: 'Usage' },
  { to: 'activity', label: 'Activity' },
  { to: 'performance', label: 'Performance' },
  { to: 'users', label: 'Users' },
];

export function DashboardTabs({ base, search }: { base: string; search: string }) {
  return (
    <nav className="flex flex-wrap gap-1 border-b border-gray-200 dark:border-gray-700">
      {TABS.map((t) => (
        <NavLink
          key={t.to || 'index'}
          to={{ pathname: t.to ? `${base}/${t.to}` : base, search }}
          end={t.end}
          className={({ isActive }) =>
            `-mb-px border-b-2 px-3 py-2 text-sm font-medium transition ${
              isActive
                ? 'border-indigo-600 text-indigo-700 dark:border-indigo-400 dark:text-indigo-400'
                : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
            }`
          }
        >
          {t.label}
        </NavLink>
      ))}
    </nav>
  );
}
