import { useState, useEffect } from 'react';
import { getUsage } from '../api/client';
import type { UsageStats } from '../types';

export function useMetrics(days: number) {
  const [stats, setStats] = useState<UsageStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    getUsage(days)
      .then((s) => {
        if (active) {
          setStats(s);
          setError('');
        }
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : 'Failed to load usage');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [days]);

  return { stats, loading, error };
}
