import { useState, useEffect } from 'react';
import { getMe, updateProfile, changePassword } from '../api/client';
import type { User } from '../types';

const inputClass =
  'w-full rounded-xl border border-gray-200 px-3 py-2.5 text-sm text-gray-900 outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200';

export function Profile() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    getMe()
      .then((u) => {
        if (active) setUser(u);
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
        <div className="mt-6 grid max-w-3xl gap-6 lg:grid-cols-2">
          <DetailsCard user={user} onSaved={setUser} />
          <PasswordCard />
        </div>
      ) : null}
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
    <form onSubmit={save} className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-gray-900">Details</h2>
        <span className="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium capitalize text-gray-500">
          {user.role}
        </span>
      </div>
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
        disabled={busy}
        className="mt-4 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:opacity-50"
      >
        {busy ? 'Saving…' : 'Save changes'}
      </button>
    </form>
  );
}

function PasswordCard() {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMsg(null);
    try {
      await changePassword(current, next);
      setMsg({ ok: true, text: 'Password updated.' });
      setCurrent('');
      setNext('');
    } catch (err) {
      setMsg({ ok: false, text: err instanceof Error ? err.message : 'Failed to update password' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm">
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
      </div>
      {msg && (
        <p className={`mt-3 text-sm ${msg.ok ? 'text-green-600' : 'text-red-600'}`}>{msg.text}</p>
      )}
      <button
        type="submit"
        disabled={busy || !current || next.length < 8}
        className="mt-4 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Update password
      </button>
    </form>
  );
}
