import { useState, type FormEvent } from 'react';

interface LoginProps {
  mode: 'login' | 'setup';
  onSubmit: (email: string, password: string) => void;
  error: string;
  loading: boolean;
  // Shown as a "Forgot password?" link in login mode; omitted for setup.
  onForgot?: () => void;
}

export function Login({ mode, onSubmit, error, loading, onForgot }: LoginProps) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  const isSetup = mode === 'setup';
  const canSubmit = email.trim() && password.length >= (isSetup ? 8 : 1);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (canSubmit) onSubmit(email.trim(), password);
  };

  const inputClass =
    'w-full px-4 py-3 rounded-xl border border-gray-200 dark:border-gray-700 dark:bg-gray-900 focus:border-indigo-500 dark:focus:border-indigo-400 focus:ring-2 focus:ring-indigo-200 dark:focus:ring-indigo-500/30 outline-none transition text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500';

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-100 dark:bg-gray-900">
      <div className="w-full max-w-sm p-8 bg-white dark:bg-gray-800 rounded-2xl border border-gray-200 dark:border-gray-700">
        <div className="text-center mb-8">
          <div className="w-16 h-16 bg-indigo-100 dark:bg-indigo-500/15 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <svg
              className="w-8 h-8 text-indigo-600 dark:text-indigo-400"
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
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-gray-50">
            {isSetup ? 'Create admin account' : 'Personal Assistant'}
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
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
          <div className="relative">
            <input
              type={showPassword ? 'text' : 'password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={isSetup ? 'Password (min 8 characters)' : 'Password'}
              autoComplete={isSetup ? 'new-password' : 'current-password'}
              className={`${inputClass} pr-11`}
            />
            <button
              type="button"
              onClick={() => setShowPassword((v) => !v)}
              aria-label={showPassword ? 'Hide password' : 'Show password'}
              aria-pressed={showPassword}
              className="absolute inset-y-0 right-0 flex items-center px-3 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 focus:outline-none focus-visible:text-indigo-600 dark:focus-visible:text-indigo-400 transition"
            >
              {showPassword ? (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"
                  />
                </svg>
              ) : (
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                  />
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                  />
                </svg>
              )}
            </button>
          </div>

          {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

          <button
            type="submit"
            disabled={loading || !canSubmit}
            className="w-full py-3 bg-indigo-600 dark:bg-indigo-500 text-white rounded-xl font-medium hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50 disabled:cursor-not-allowed transition"
          >
            {loading
              ? isSetup
                ? 'Creating…'
                : 'Signing in…'
              : isSetup
                ? 'Create account'
                : 'Sign in'}
          </button>

          {!isSetup && onForgot && (
            <button
              type="button"
              onClick={onForgot}
              className="w-full text-center text-sm text-indigo-700 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 hover:underline focus:outline-none transition"
            >
              Forgot password?
            </button>
          )}
        </form>
      </div>
    </div>
  );
}
