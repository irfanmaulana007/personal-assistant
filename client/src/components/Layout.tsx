import { useState, type ReactNode } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { UserMenu } from './UserMenu';
import { ProjectSwitcher } from './ProjectSwitcher';
import { APP_VERSION_LABEL } from '../appVersion';
import type { User } from '../types';

interface LayoutProps {
  onLogout: () => void;
  isAdmin: boolean;
  user: User | null;
}

interface NavLeaf {
  to: string;
  label: string;
  icon: ReactNode;
  adminOnly?: boolean;
}
interface NavGroupItem {
  label: string;
  icon: ReactNode;
  adminOnly?: boolean;
  children: { to: string; label: string; end?: boolean }[];
}
type NavEntry = NavLeaf | NavGroupItem;

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

const navItems: NavEntry[] = [
  { to: '/chat', label: 'Chat', icon: chatIcon },
  {
    label: 'Dashboard',
    adminOnly: true,
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
      />
    ),
    children: [
      { to: '/dashboard', label: 'Overview', end: true },
      { to: '/dashboard/projects', label: 'Projects' },
      { to: '/dashboard/usage', label: 'Usage' },
      { to: '/dashboard/activity', label: 'Activity' },
      { to: '/dashboard/performance', label: 'Performance' },
      { to: '/dashboard/users', label: 'Users' },
    ],
  },
  {
    to: '/reminders',
    label: 'Reminders',
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
    to: '/projects',
    label: 'Projects',
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
      />
    ),
  },
  {
    to: '/integrations',
    label: 'Integrations',
    adminOnly: true,
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
    adminOnly: true,
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"
      />
    ),
  },
  {
    to: '/account',
    label: 'Account',
    adminOnly: true,
    icon: (
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
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

function NavGroup({ item }: { item: NavGroupItem }) {
  const location = useLocation();
  const anyActive = item.children.some((c) =>
    c.end ? location.pathname === c.to : location.pathname.startsWith(c.to),
  );
  const [open, setOpen] = useState(anyActive);

  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className={`w-full ${leafClass(anyActive)}`}
      >
        <Icon>{item.icon}</Icon>
        <span className="flex-1 text-left">{item.label}</span>
        <svg
          className={`h-4 w-4 shrink-0 text-slate-500 transition-transform ${open ? 'rotate-90' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
      </button>
      {open && (
        <div className="mt-0.5 space-y-0.5 pl-4">
          {item.children.map((c) => (
            <NavLink
              key={c.to}
              to={c.to}
              end={c.end}
              className={({ isActive }) =>
                `block rounded-lg px-3 py-1.5 text-sm transition ${
                  isActive
                    ? 'font-medium text-white'
                    : 'font-normal text-slate-400 hover:text-slate-100'
                }`
              }
            >
              {c.label}
            </NavLink>
          ))}
        </div>
      )}
    </div>
  );
}

export function Layout({ onLogout, isAdmin, user }: LayoutProps) {
  const items = navItems.filter((item) => isAdmin || !item.adminOnly);

  return (
    <div className="flex h-screen bg-gray-100 dark:bg-gray-900">
      <aside className="flex w-60 shrink-0 flex-col bg-slate-900 text-slate-300 dark:border-r dark:border-white/5">
        <div className="px-3 pb-1 pt-4">
          <ProjectSwitcher isSuperadmin={isAdmin} />
        </div>

        <nav className="flex-1 space-y-0.5 px-3 py-2">
          {items.map((item) =>
            'children' in item ? (
              <NavGroup key={item.label} item={item} />
            ) : (
              <NavLink key={item.to} to={item.to} className={({ isActive }) => leafClass(isActive)}>
                <Icon>{item.icon}</Icon>
                {item.label}
              </NavLink>
            ),
          )}
        </nav>

        <div className="px-3 pb-2">
          <NavLink to="/settings" className={({ isActive }) => leafClass(isActive)}>
            <Icon>{settingsIcon}</Icon>
            Settings
          </NavLink>
        </div>

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
