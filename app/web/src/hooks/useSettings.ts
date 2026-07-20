import { useState, useEffect, useCallback } from 'react';
import { getSettings, updateSettings, testSettings } from '../api/client';
import type { LlmSettings, LlmSettingsUpdate, LlmTestResult } from '../types';

export function useSettings() {
  const [settings, setSettings] = useState<LlmSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    getSettings()
      .then((s) => {
        if (active) setSettings(s);
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : 'Failed to load settings');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const save = useCallback(async (update: LlmSettingsUpdate): Promise<boolean> => {
    setError('');
    try {
      setSettings(await updateSettings(update));
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings');
      return false;
    }
  }, []);

  const test = useCallback(async (): Promise<LlmTestResult> => {
    try {
      return await testSettings();
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : 'Test failed' };
    }
  }, []);

  return { settings, loading, error, save, test };
}
