import { useState, useEffect, useCallback } from 'react';
import {
  login as apiLogin,
  setupAdmin as apiSetup,
  getAuthStatus,
  getMe,
  clearToken,
  isAuthenticated,
} from '../api/client';
import type { User } from '../types';

export function useAuth() {
  const [user, setUser] = useState<User | null>(null);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const status = await getAuthStatus();
        if (status.setup_required) {
          if (active) setNeedsSetup(true);
          return;
        }
        if (isAuthenticated()) {
          try {
            const me = await getMe();
            if (active) setUser(me);
          } catch {
            clearToken();
          }
        }
      } catch {
        // network error — fall through to login screen
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    setSubmitting(true);
    setError('');
    try {
      const res = await apiLogin(email, password);
      setUser(res.user);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setSubmitting(false);
    }
  }, []);

  const setup = useCallback(async (email: string, password: string) => {
    setSubmitting(true);
    setError('');
    try {
      const res = await apiSetup(email, password);
      setUser(res.user);
      setNeedsSetup(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed');
    } finally {
      setSubmitting(false);
    }
  }, []);

  const logout = useCallback(() => {
    clearToken();
    setUser(null);
  }, []);

  return {
    user,
    authenticated: user !== null,
    // Global platform admin is now the superadmin role; isAdmin keeps its name so
    // existing platform-admin gating (settings, users, …) tracks superadmin.
    isAdmin: user?.role === 'superadmin',
    isSuperadmin: user?.role === 'superadmin',
    needsSetup,
    loading,
    submitting,
    error,
    login,
    setup,
    logout,
  };
}
