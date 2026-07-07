import { useState, useCallback } from 'react';
import { login as apiLogin, clearToken, isAuthenticated } from '../api/client';

export function useAuth() {
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const login = useCallback(async (password: string) => {
    setLoading(true);
    setError('');
    try {
      await apiLogin(password);
      setAuthenticated(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  }, []);

  const logout = useCallback(() => {
    clearToken();
    setAuthenticated(false);
  }, []);

  return { authenticated, login, logout, error, loading };
}
