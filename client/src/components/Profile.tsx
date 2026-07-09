import { useState, useEffect } from 'react';
import { format, parseISO } from 'date-fns';
import { getMe, getMyStats, updateProfile, changePassword } from '../api/client';
import { formatTokens } from '../lib/format';
import type { User, MyStats } from '../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

export function Profile() {
  const [user, setUser] = useState<User | null>(null);
  const [stats, setStats] = useState<MyStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    Promise.all([getMe(), getMyStats().catch(() => null)])
      .then(([u, s]) => {
        if (active) {
          setUser(u);
          setStats(s);
        }
      })
      .catch(() => {})
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="flex-1 overflow-y-auto bg-gray-50 p-6">
      <h1 className="text-xl font-semibold tracking-tight text-gray-900">Profile</h1>
      <p className="mt-0.5 text-sm text-gray-500">Manage your account details and password.</p>

      {loading ? (
        <p className="mt-6 text-sm text-gray-500">Loading…</p>
      ) : user ? (
        <div className="mt-6 max-w-4xl space-y-6">
          <ProfileHeader user={user} />
          {stats && <ActivityRow stats={stats} />}
          <div className="grid gap-6 lg:grid-cols-2">
            <DetailsCard user={user} onSaved={setUser} />
            <PasswordCard />
          </div>
        </div>
      ) : null}
    </div>
  );
}

function ProfileHeader({ user }: { user: User }) {
  const displayName = user.name?.trim() || user.email;
  const initial = (user.name?.trim() || user.email || '?').charAt(0).toUpperCase();
  return (
    <div className="flex flex-wrap items-center gap-4 rounded-2xl border border-gray-200 bg-white p-5">
      <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-indigo-100 text-2xl font-semibold text-indigo-700">
        {initial}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <h2 className="truncate text-lg font-semibold text-gray-900">{displayName}</h2>
          <span
            className={`rounded-full px-2.5 py-0.5 text-xs font-medium capitalize ${
              user.role === 'admin' ? 'bg-indigo-100 text-indigo-700' : 'bg-gray-100 text-gray-500'
            }`}
          >
            {user.role}
          </span>
        </div>
        <p className="truncate text-sm text-gray-500">{user.email}</p>
        <p className="mt-0.5 text-xs text-gray-400">
          Joined {format(parseISO(user.created_at), 'MMMM d, yyyy')}
        </p>
      </div>
    </div>
  );
}

function ActivityRow({ stats }: { stats: MyStats }) {
  const tiles = [
    { label: 'Conversations', value: stats.runs.toLocaleString() },
    { label: 'Tokens used', value: formatTokens(stats.total_tokens) },
    { label: 'Active reminders', value: stats.reminders.toLocaleString() },
    { label: 'Notes', value: stats.notes.toLocaleString() },
  ];
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {tiles.map((t) => (
        <div key={t.label} className="rounded-2xl border border-gray-200 bg-white p-4">
          <div className="text-[11px] font-medium uppercase tracking-wide text-gray-400">
            {t.label}
          </div>
          <div className="mt-1.5 text-2xl font-semibold tracking-tight text-gray-900 tabular-nums">
            {t.value}
          </div>
        </div>
      ))}
    </div>
  );
}

function DetailsCard({ user, onSaved }: { user: User; onSaved: (u: User) => void }) {
  const [name, setName] = useState<string | null>(null);
  const [email, setEmail] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const nameVal = name ?? user.name ?? '';
  const emailVal = email ?? user.email;
  const dirty = nameVal.trim() !== (user.name ?? '') || emailVal.trim() !== user.email;

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMsg(null);
    try {
      const updated = await updateProfile(nameVal.trim(), emailVal.trim());
      onSaved(updated);
      setName(null);
      setEmail(null);
      setMsg({ ok: true, text: 'Profile updated.' });
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to update profile' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={save} className="rounded-2xl border border-gray-200 bg-white p-5">
      <h2 className="mb-4 text-sm font-semibold text-gray-900">Details</h2>
      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Name</label>
          <input
            value={nameVal}
            onChange={(e) => setName(e.target.value)}
            placeholder="Your name"
            className={inputClass}
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-gray-700">Email</label>
          <input
            type="email"
            value={emailVal}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            className={inputClass}
          />
          <p className="mt-1 text-xs text-gray-400">You sign in with this email.</p>
        </div>
      </div>
      {msg && (
        <p className={`mt-3 text-sm ${msg.ok ? 'text-green-600' : 'text-red-600'}`}>{msg.text}</p>
      )}
      <button
        type="submit"
        disabled={busy || !dirty}
        className="mt-4 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Saving…' : 'Save changes'}
      </button>
    </form>
  );
}

function PasswordCard() {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const mismatch = confirm.length > 0 && next !== confirm;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (next !== confirm) {
      setMsg({ ok: false, text: 'New passwords do not match.' });
      return;
    }
    setBusy(true);
    setMsg(null);
    try {
      await changePassword(current, next);
      setMsg({ ok: true, text: 'Password updated.' });
      setCurrent('');
      setNext('');
      setConfirm('');
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to update password' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="rounded-2xl border border-gray-200 bg-white p-5">
      <h2 className="mb-4 text-sm font-semibold text-gray-900">Password</h2>
      <div className="space-y-3">
        <input
          type="password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          placeholder="Current password"
          autoComplete="current-password"
          className={inputClass}
        />
        <input
          type="password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          placeholder="New password (min 8)"
          autoComplete="new-password"
          className={inputClass}
        />
        <input
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          placeholder="Confirm new password"
          autoComplete="new-password"
          className={inputClass}
        />
      </div>
      {mismatch && <p className="mt-2 text-xs text-red-600">Passwords don't match.</p>}
      {msg && (
        <p className={`mt-3 text-sm ${msg.ok ? 'text-green-600' : 'text-red-600'}`}>{msg.text}</p>
      )}
      <button
        type="submit"
        disabled={busy || !current || next.length < 8 || next !== confirm}
        className="mt-4 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Update password
      </button>
    </form>
  );
}
