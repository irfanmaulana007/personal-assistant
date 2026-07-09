import { useState, useEffect } from 'react';
import { listUsers, createUser, updateUser, deleteUser, getMe } from '../api/client';
import type { User, Role } from '../types';

const inputClass =
  'rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

export function Account() {
  const [users, setUsers] = useState<User[]>([]);
  const [me, setMe] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);

  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    let active = true;
    Promise.all([listUsers(), getMe()])
      .then(([u, m]) => {
        if (active) {
          setUsers(u);
          setMe(m);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : 'Failed to load users');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [refreshKey]);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-50 p-6">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900">Account</h1>
      <p className="mt-0.5 text-sm text-gray-500">Manage users and their roles.</p>

      {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

      {loading ? (
        <p className="mt-6 text-sm text-gray-500">Loading…</p>
      ) : (
        <UsersCard users={users} meId={me?.id ?? 0} onChanged={reload} />
      )}
    </div>
  );
}

function UsersCard({
  users,
  meId,
  onChanged,
}: {
  users: User[];
  meId: number;
  onChanged: () => void | Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [rowError, setRowError] = useState('');

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setRowError('');
    try {
      await fn();
      await onChanged();
    } catch (e) {
      setRowError(e instanceof Error ? e.message : 'Action failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-6 rounded-2xl border border-gray-200 bg-white p-5">
      <h2 className="mb-4 text-sm font-semibold text-gray-900">Users</h2>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-100 text-left text-xs font-medium uppercase tracking-wide text-gray-400">
              <th className="pb-2 font-medium">Email</th>
              <th className="pb-2 font-medium">Role</th>
              <th className="pb-2 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} className="border-b border-gray-50 last:border-0">
                <td className="py-3 text-gray-800">
                  {u.email}
                  {u.id === meId && <span className="ml-2 text-xs text-gray-400">(you)</span>}
                </td>
                <td className="py-3">
                  <select
                    value={u.role}
                    disabled={busy}
                    onChange={(e) => run(() => updateUser(u.id, { role: e.target.value as Role }))}
                    className="rounded-lg border border-gray-200 px-2 py-1 text-sm outline-none focus:border-indigo-500"
                  >
                    <option value="admin">admin</option>
                    <option value="member">member</option>
                  </select>
                </td>
                <td className="py-3 text-right">
                  {u.id !== meId && (
                    <button
                      disabled={busy}
                      onClick={() => run(() => deleteUser(u.id))}
                      className="rounded-lg px-2.5 py-1 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50"
                    >
                      Delete
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {rowError && <p className="mt-3 text-sm text-red-600">{rowError}</p>}

      <AddUserForm
        busy={busy}
        onAdd={(email, password, role) => run(() => createUser(email, password, role))}
      />
    </div>
  );
}

function AddUserForm({
  busy,
  onAdd,
}: {
  busy: boolean;
  onAdd: (email: string, password: string, role: Role) => void;
}) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<Role>('member');

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (email.trim() && password.length >= 8) {
      onAdd(email.trim(), password, role);
      setEmail('');
      setPassword('');
      setRole('member');
    }
  };

  return (
    <form
      onSubmit={submit}
      className="mt-5 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-4"
    >
      <input
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="new.user@email.com"
        className={`${inputClass} flex-1 min-w-[180px]`}
      />
      <input
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        placeholder="Password (min 8)"
        autoComplete="new-password"
        className={`${inputClass} w-44`}
      />
      <select value={role} onChange={(e) => setRole(e.target.value as Role)} className={inputClass}>
        <option value="member">member</option>
        <option value="admin">admin</option>
      </select>
      <button
        type="submit"
        disabled={busy || !email.trim() || password.length < 8}
        className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Add user
      </button>
    </form>
  );
}
