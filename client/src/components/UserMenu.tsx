import * as Popover from '@radix-ui/react-popover';
import { useNavigate } from 'react-router-dom';
import type { User } from '../types';

const itemClass =
  'flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm font-medium transition';

export function UserMenu({ user, onLogout }: { user: User | null; onLogout: () => void }) {
  const navigate = useNavigate();
  const displayName = user?.name?.trim() || user?.email || 'Account';
  const initial = (user?.name?.trim() || user?.email || '?').charAt(0).toUpperCase();
  const showEmailLine = Boolean(user?.email && user?.name?.trim());

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left transition hover:bg-white/5">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-indigo-500 text-sm font-semibold text-white">
            {initial}
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium text-white">{displayName}</div>
            {showEmailLine && <div className="truncate text-xs text-slate-400">{user?.email}</div>}
          </div>
          <svg
            className="h-4 w-4 shrink-0 text-slate-500"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M8 9l4-4 4 4m0 6l-4 4-4-4"
            />
          </svg>
        </button>
      </Popover.Trigger>

      <Popover.Portal>
        <Popover.Content
          side="top"
          align="start"
          sideOffset={8}
          className="z-50 w-[228px] rounded-xl border border-gray-200 bg-white p-1 shadow-lg"
        >
          <div className="border-b border-gray-100 px-3 py-2">
            <div className="truncate text-sm font-medium text-gray-900">{displayName}</div>
            <div className="truncate text-xs capitalize text-gray-400">
              {user?.email}
              {user?.role ? ` · ${user.role}` : ''}
            </div>
          </div>
          <div className="p-1">
            <Popover.Close asChild>
              <button
                onClick={() => navigate('/profile')}
                className={`${itemClass} text-gray-700 hover:bg-gray-100`}
              >
                <svg
                  className="h-4 w-4 text-gray-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                  />
                </svg>
                Profile
              </button>
            </Popover.Close>
            <Popover.Close asChild>
              <button onClick={onLogout} className={`${itemClass} text-red-600 hover:bg-red-50`}>
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                  />
                </svg>
                Log out
              </button>
            </Popover.Close>
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
