import { type ReactNode } from 'react';
import { NavLink, Navigate, Outlet, useParams } from 'react-router-dom';
import { UserMenu } from './UserMenu';
import { ProjectSwitcher } from './ProjectSwitcher';
import { APP_VERSION_LABEL } from '../appVersion';
import { useProjects } from '../contexts/project';
import type { User } from '../types';

interface LayoutProps {
  onLogout: () => void;
  isAdmin: boolean;
  user: User | null;
  // 'global'  — the platform shell (Dashboard / Account / Projects / Settings).
  // 'project' — a single project's shell, prefixed by /:slug.
  mode: 'global' | 'project';
}

// Who can see a nav item:
//   'everyone'      — any authenticated user
//   'projectAdmin'  — admin of the active project (superadmin always qualifies)
//   'superadmin'    — global superadmin only
type NavGate = 'everyone' | 'projectAdmin' | 'superadmin';

interface NavLeaf {
  to: string;
  label: string;
  icon: ReactNode;
  gate?: NavGate;
  // When set, the item is a project feature — shown only if that feature (and,
  // if it owns skills, at least one of its skills) is enabled for the project.
  feature?: string;
}

const chatIcon = (
  <path
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth={2}
    d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z"
  />
);

const settingsIcon = (
  <>
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
    />
    <path
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
      d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
    />
  </>
);

const accountIcon = (
  <path
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth={2}
    d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
  />
);

const overviewIcon = (
  <path
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth={2}
    d="M4 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM14 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1V5zM4 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1v-4zM14 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z"
  />
);

const projectsIcon = (
  <path
    strokeLinecap="round"
    strokeLinejoin="round"
    strokeWidth={2}
    d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
  />
);

// Global platform surfaces (the global shell). Dashboard / Account are superadmin
// only; Projects is every member's picker into their project shells. Settings is
// pinned to the bottom (see globalSettingsItem), mirroring the project shell.
const globalNavItems: NavLeaf[] = [
  { to: '/dashboard', label: 'Dashboard', gate: 'superadmin', icon: overviewIcon },
  { to: '/account', label: 'Account', gate: 'superadmin', icon: accountIcon },
  { to: '/projects', label: 'Projects', gate: 'everyone', icon: projectsIcon },
];

// Settings sits at the bottom of the sidebar in both shells, above the user menu.
const globalSettingsItem: NavLeaf = {
  to: '/settings',
  label: 'Settings',
  gate: 'superadmin',
  icon: settingsIcon,
};

// Project-scoped nav. Paths are relative to the project root (leading '/') and
// get the active project's /:slug prefix applied before rendering.
const navItems: NavLeaf[] = [
  { to: '/chat', label: 'Chat', icon: chatIcon, gate: 'everyone' },
  {
    to: '/dashboard',
    label: 'Dashboard',
    gate: 'projectAdmin',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
      />
    ),
  },
  {
    to: '/reminders',
    label: 'Reminders',
    feature: 'reminders',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"
      />
    ),
  },
  {
    to: '/bucket-list',
    label: 'Bucket List',
    feature: 'bucket_list',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7l2 2 4-4"
      />
    ),
  },
  {
    to: '/skills',
    label: 'Skills',
    gate: 'projectAdmin',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M5 3v4M3 5h4M6 17v4m-2-2h4m6-14l2.4 6.6L22 13l-6.6 2.4L13 22l-2.4-6.6L4 13l6.6-2.4L13 3z"
      />
    ),
  },
  {
    to: '/workflow',
    label: 'Workflow',
    gate: 'superadmin',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
      />
    ),
  },
  {
    to: '/integrations',
    label: 'Integrations',
    gate: 'projectAdmin',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M13 10V3L4 14h7v7l9-11h-7z"
      />
    ),
  },
  {
    to: '/logs',
    label: 'Logs',
    gate: 'projectAdmin',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"
      />
    ),
  },
];

function leafClass(isActive: boolean) {
  return `flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
    isActive
      ? 'bg-white/10 font-medium text-white'
      : 'font-normal text-slate-400 hover:bg-white/5 hover:text-slate-100'
  }`;
}

function Icon({ children }: { children: ReactNode }) {
  return (
    <svg className="h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      {children}
    </svg>
  );
}

export function Layout({ onLogout, isAdmin, user, mode }: LayoutProps) {
  const { canManageActive, navFeatureVisible, projects, loading } = useProjects();
  const { slug } = useParams();

  const canSee = (gate: NavGate | undefined): boolean => {
    switch (gate ?? 'everyone') {
      case 'superadmin':
        return isAdmin; // isAdmin === global superadmin
      case 'projectAdmin':
        return canManageActive;
      default:
        return true;
    }
  };

  // In a project shell, a slug that matches no project the caller can see is a
  // dead URL — bounce home rather than render someone else's project chrome.
  if (mode === 'project' && !loading && slug && !projects.some((p) => p.slug === slug)) {
    return <Navigate to="/" replace />;
  }

  const withSlug = (to: string) => `/${slug}${to}`;

  const items = navItems
    .filter((item) => {
      if (!canSee(item.gate)) return false;
      if (item.feature) return navFeatureVisible(item.feature);
      return true;
    })
    .map((item) => ({ ...item, to: withSlug(item.to) }));

  const globalItems = globalNavItems.filter((item) => canSee(item.gate));

  return (
    <div className="flex h-screen bg-gray-100 dark:bg-gray-900">
      <aside className="flex w-60 shrink-0 flex-col bg-slate-900 text-slate-300 dark:border-r dark:border-white/5">
        {mode === 'global' ? (
          <>
            <nav className="flex-1 space-y-0.5 px-3 py-4">
              {globalItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) => leafClass(isActive)}
                >
                  <Icon>{item.icon}</Icon>
                  {item.label}
                </NavLink>
              ))}
            </nav>

            {canSee(globalSettingsItem.gate) && (
              <div className="px-3 pb-2">
                <NavLink
                  to={globalSettingsItem.to}
                  end
                  className={({ isActive }) => leafClass(isActive)}
                >
                  <Icon>{globalSettingsItem.icon}</Icon>
                  {globalSettingsItem.label}
                </NavLink>
              </div>
            )}
          </>
        ) : (
          <>
            {isAdmin && (
              <div className="px-3 pt-3">
                <NavLink
                  to="/dashboard"
                  className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-normal text-slate-500 transition hover:bg-white/5 hover:text-slate-300"
                >
                  <svg
                    className="h-3.5 w-3.5 shrink-0"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M11 17l-5-5m0 0l5-5m-5 5h12"
                    />
                  </svg>
                  All projects
                </NavLink>
              </div>
            )}

            <div className={`px-3 pb-1 ${isAdmin ? 'pt-3' : 'pt-4'}`}>
              <ProjectSwitcher isSuperadmin={isAdmin} />
            </div>

            <nav className="flex-1 space-y-0.5 px-3 py-2">
              {items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) => leafClass(isActive)}
                >
                  <Icon>{item.icon}</Icon>
                  {item.label}
                </NavLink>
              ))}
            </nav>

            <div className="px-3 pb-2">
              <NavLink to={withSlug('/settings')} className={({ isActive }) => leafClass(isActive)}>
                <Icon>{settingsIcon}</Icon>
                Settings
              </NavLink>
            </div>
          </>
        )}

        <div className="border-t border-white/10 p-2">
          <UserMenu user={user} onLogout={onLogout} />
          <p className="px-2 pt-1.5 text-[11px] font-medium tracking-wide text-white/40">
            {APP_VERSION_LABEL}
          </p>
        </div>
      </aside>

      <main className="flex flex-1 flex-col overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}
