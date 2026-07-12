import { useState, useEffect } from 'react';
import { getUsage } from '../api/client';
import type { UsageStats, ChannelValue } from '../types';

export function useMetrics(from: string, to: string, platforms: ChannelValue[]) {
  const [stats, setStats] = useState<UsageStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Join the platforms so the effect re-runs on selection change (arrays are
  // compared by identity, which would otherwise refetch on every render).
  const platformKey = platforms.join(',');

  useEffect(() => {
    let active = true;
    getUsage(from, to, platforms)
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [from, to, platformKey]);

  return { stats, loading, error };
}
