import { useState, type FormEvent } from 'react';

interface LoginProps {
  mode: 'login' | 'setup';
  onSubmit: (email: string, password: string) => void;
  error: string;
  loading: boolean;
}

export function Login({ mode, onSubmit, error, loading }: LoginProps) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const isSetup = mode === 'setup';
  const canSubmit = email.trim() && password.length >= (isSetup ? 8 : 1);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (canSubmit) onSubmit(email.trim(), password);
  };

  const inputClass =
    'w-full px-4 py-3 rounded-xl border border-gray-200 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 outline-none transition text-gray-900 placeholder-gray-400';

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="w-full max-w-sm p-8 bg-white rounded-2xl shadow-lg">
        <div className="text-center mb-8">
          <div className="w-16 h-16 bg-indigo-100 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <svg
              className="w-8 h-8 text-indigo-600"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z"
              />
            </svg>
          </div>
          <h1 className="text-2xl font-semibold text-gray-900">
            {isSetup ? 'Create admin account' : 'Personal Assistant'}
          </h1>
          <p className="text-sm text-gray-500 mt-1">
            {isSetup ? 'Set up the first account to get started' : 'Sign in to continue'}
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3">
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Email"
            autoFocus
            autoComplete="username"
            className={inputClass}
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={isSetup ? 'Password (min 8 characters)' : 'Password'}
            autoComplete={isSetup ? 'new-password' : 'current-password'}
            className={inputClass}
          />

          {error && <p className="text-sm text-red-600">{error}</p>}

          <button
            type="submit"
            disabled={loading || !canSubmit}
            className="w-full py-3 bg-indigo-600 text-white rounded-xl font-medium hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed transition"
          >
            {loading
              ? isSetup
                ? 'Creating…'
                : 'Signing in…'
              : isSetup
                ? 'Create account'
                : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
